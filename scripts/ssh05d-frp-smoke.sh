#!/usr/bin/env bash
# Real supervisor FRP smoke: managed frps + frpc with TCP, UDP, STCP and XTCP.
# Requires bin/frps and bin/frpc (`make download-frp`), python3 and bun.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
DIR="$(mktemp -d "${TMPDIR:-/tmp}/ssh05d-frp-smoke.XXXXXX")"
SSH05D_PID=""
HTTP_PID=""
UDP_PID=""
GO="${GO:-go}"
BUN="${BUN:-bun}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"
[[ -x "$HOME/.bun/bin/bun" && "$BUN" = "bun" ]] && BUN="$HOME/.bun/bin/bun"

cleanup() {
  [[ -n "$SSH05D_PID" ]] && kill "$SSH05D_PID" 2>/dev/null || true
  [[ -n "$HTTP_PID" ]] && kill "$HTTP_PID" 2>/dev/null || true
  [[ -n "$UDP_PID" ]] && kill "$UDP_PID" 2>/dev/null || true
  [[ -n "${SSH05_KEEP_DIR:-}" ]] || rm -rf "$DIR"
}
trap cleanup EXIT
[[ -n "${SSH05_KEEP_DIR:-}" ]] && echo "keep: $DIR"

fail() {
  echo "FAIL: $*" >&2
  echo "--- ssh05d ---" >&2
  tail -40 "$DIR/ssh05d.log" 2>/dev/null >&2 || true
  echo "--- frps ---" >&2
  cat "$DIR"/state/ssh05/frp/*-frps.log 2>/dev/null >&2 || true
  echo "--- frpc ---" >&2
  cat "$DIR"/state/ssh05/frp/*.log 2>/dev/null >&2 || true
  echo "--- http ---" >&2
  cat "$DIR/http.log" 2>/dev/null >&2 || true
  echo "--- udp ---" >&2
  cat "$DIR/udp.log" 2>/dev/null >&2 || true
  exit 1
}

[[ -x bin/frps && -x bin/frpc ]] || fail "missing bin/frps or bin/frpc; run make download-frp"
command -v python3 >/dev/null || fail "python3 required"
command -v "$BUN" >/dev/null || fail "bun required"

echo "SSH05_FRP_SUPERVISOR_OK" >"$DIR/hello.txt"
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
for _ in $(seq 1 50); do
  curl -fsS --max-time 1 http://127.0.0.1:18080/hello.txt >/dev/null 2>&1 && break
  sleep 0.1
done

CGO_ENABLED=0 "$GO" build -o "$DIR/ssh05d" ./cmd/ssh05d
XDG_STATE_HOME="$DIR/state" "$DIR/ssh05d" --socket "$DIR/control.sock" --ssh-config /dev/null \
  --audit-log "$DIR/audit.log" --frps-listen 0.0.0.0:17000 \
  --frpc "$ROOT/bin/frpc" --frps "$ROOT/bin/frps" --frp-config "$DIR/frp.json" \
  >"$DIR/ssh05d.log" 2>&1 &
SSH05D_PID=$!

for _ in $(seq 1 100); do
  [[ -S "$DIR/control.sock" ]] && break
  sleep 0.1
done
[[ -S "$DIR/control.sock" ]] || fail "supervisor socket did not appear"

SSH05_SOCKET="$DIR/control.sock" "$BUN" prototype/terminal/scripts/ssh05d-frp-smoke.ts || fail "FRP smoke failed"

echo "PASS: managed frps + frpc with TCP/UDP/STCP (XTCP conditional)"
