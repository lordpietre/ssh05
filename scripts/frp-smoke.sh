#!/usr/bin/env bash
# Real FRP smoke test: frps + frpc + TCP proxy + curl.
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

cleanup() {
  [[ -n "$FRPC_PID" ]] && kill "$FRPC_PID" 2>/dev/null || true
  [[ -n "$FRPS_PID" ]] && kill "$FRPS_PID" 2>/dev/null || true
  [[ -n "$HTTP_PID" ]] && kill "$HTTP_PID" 2>/dev/null || true
  rm -rf "$DIR"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  cat "$DIR/frps.log" "$DIR/frpc.log" >&2 2>/dev/null || true
  exit 1
}

[[ -x "$FRPS" ]] || fail "missing $FRPS; run make download-frp"
[[ -x "$FRPC" ]] || fail "missing $FRPC; run make download-frp"
command -v python3 >/dev/null || fail "python3 required"
command -v curl >/dev/null || fail "curl required"

TOKEN="$(python3 -c 'import secrets; print(secrets.token_hex(16))')"
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
name = "trss-smoke"
type = "tcp"
localIP = "127.0.0.1"
localPort = 18080
remotePort = 18081
EOF

"$FRPS" verify -c "$DIR/frps.toml" >/dev/null
"$FRPC" verify -c "$DIR/frpc.toml" >/dev/null

python3 -m http.server 18080 --bind 127.0.0.1 --directory "$DIR" >"$DIR/http.log" 2>&1 &
HTTP_PID=$!
"$FRPS" -c "$DIR/frps.toml" >"$DIR/frps.log" 2>&1 &
FRPS_PID=$!
sleep 0.5
"$FRPC" -c "$DIR/frpc.toml" >"$DIR/frpc.log" 2>&1 &
FRPC_PID=$!

for _ in $(seq 1 50); do
  if body="$(curl -fsS --max-time 1 http://127.0.0.1:18081/hello.txt 2>/dev/null)"; then
    if [[ "$body" == "TRSS_FRP_OK" ]]; then
      echo "PASS: frps/frpc TCP tunnel returns the expected payload"
      exit 0
    fi
    fail "unexpected payload: $body"
  fi
  sleep 0.1
done
fail "TCP tunnel did not become reachable"
