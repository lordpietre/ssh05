#!/usr/bin/env bash
# Real FRP smoke test: frps + frpc + TCP/STCP/UDP proxies.
# Requires bin/frps and bin/frpc (run `make download-frp`).
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
FRPS="${FRPS:-$ROOT/bin/frps}"
FRPC="${FRPC:-$ROOT/bin/frpc}"
DIR="$(mktemp -d "${TMPDIR:-/tmp}/trss-frp-smoke.XXXXXX")"
FRPS_PID=""
FRPC_PID=""
HTTP_PID=""
UDP_PID=""

cleanup() {
  [[ -n "$FRPC_PID" ]] && kill "$FRPC_PID" 2>/dev/null || true
  [[ -n "$FRPS_PID" ]] && kill "$FRPS_PID" 2>/dev/null || true
  [[ -n "$HTTP_PID" ]] && kill "$HTTP_PID" 2>/dev/null || true
  [[ -n "$UDP_PID" ]] && kill "$UDP_PID" 2>/dev/null || true
  rm -rf "$DIR"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  cat "$DIR/frps.log" "$DIR/frpc.log" "$DIR/http.log" "$DIR/udp.log" >&2 2>/dev/null || true
  exit 1
}

[[ -x "$FRPS" ]] || fail "missing $FRPS; run make download-frp"
[[ -x "$FRPC" ]] || fail "missing $FRPC; run make download-frp"
command -v python3 >/dev/null || fail "python3 required"
command -v curl >/dev/null || fail "curl required"

TOKEN="$(python3 -c 'import secrets; print(secrets.token_hex(16))')"
SECRET="$(python3 -c 'import secrets; print(secrets.token_hex(16))')"
echo "TRSS_FRP_OK" > "$DIR/hello.txt"

cat > "$DIR/frps.toml" <<EOF
bindPort = 17000

auth.method = "token"
auth.token = "$TOKEN"

webServer.addr = "127.0.0.1"
webServer.port = 17400
EOF

cat > "$DIR/frpc.toml" <<EOF
serverAddr = "127.0.0.1"
serverPort = 17000

auth.method = "token"
auth.token = "$TOKEN"

webServer.addr = "127.0.0.1"
webServer.port = 17401

[[proxies]]
name = "trss-smoke-tcp"
type = "tcp"
localIP = "127.0.0.1"
localPort = 18080
remotePort = 18081

[[proxies]]
name = "trss-smoke-stcp"
type = "stcp"
localIP = "127.0.0.1"
localPort = 18080
secretKey = "$SECRET"

[[visitors]]
name = "trss-smoke-stcp-visitor"
type = "stcp"
serverName = "trss-smoke-stcp"
secretKey = "$SECRET"
bindAddr = "127.0.0.1"
bindPort = 18082

[[proxies]]
name = "trss-smoke-udp"
type = "udp"
localIP = "127.0.0.1"
localPort = 18084
remotePort = 18085
EOF

"$FRPS" verify -c "$DIR/frps.toml" >/dev/null
"$FRPC" verify -c "$DIR/frpc.toml" >/dev/null

python3 -m http.server 18080 --bind 127.0.0.1 --directory "$DIR" >"$DIR/http.log" 2>&1 &
HTTP_PID=$!
python3 - "$DIR/udp.log" <<'PY' &
import socket, sys
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(("127.0.0.1", 18084))
with open(sys.argv[1], "w") as log:
    while True:
        data, addr = sock.recvfrom(65535)
        log.write(f"{addr} {data!r}\n")
        log.flush()
        sock.sendto(data, addr)
PY
UDP_PID=$!

"$FRPS" -c "$DIR/frps.toml" >"$DIR/frps.log" 2>&1 &
FRPS_PID=$!
sleep 0.5
"$FRPC" -c "$DIR/frpc.toml" >"$DIR/frpc.log" 2>&1 &
FRPC_PID=$!

wait_http() {
  local port="$1" label="$2"
  for _ in $(seq 1 50); do
    if body="$(curl -fsS --max-time 1 "http://127.0.0.1:$port/hello.txt" 2>/dev/null)"; then
      [[ "$body" == "TRSS_FRP_OK" ]] || fail "$label unexpected payload: $body"
      echo "PASS: $label"
      return 0
    fi
    sleep 0.1
  done
  fail "$label did not become reachable"
}

wait_udp() {
  for _ in $(seq 1 50); do
    if python3 - <<'PY'
import socket, sys
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.settimeout(0.3)
sock.sendto(b"TRSS_UDP_OK", ("127.0.0.1", 18085))
try:
    data, _ = sock.recvfrom(65535)
    sys.exit(0 if data == b"TRSS_UDP_OK" else 1)
except OSError:
    sys.exit(1)
PY
    then
      echo "PASS: frps/frpc UDP tunnel echoes the expected payload"
      return 0
    fi
    sleep 0.1
  done
  fail "UDP tunnel did not become reachable"
}

wait_http 18081 "frps/frpc TCP tunnel returns the expected payload"
wait_http 18082 "frps/frpc STCP tunnel returns the expected payload"
wait_udp
