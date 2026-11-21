#!/usr/bin/env bash
# Real supervisor FRP smoke: managed frps + frpc + TCP proxy through trssd.
# Requires bin/frps and bin/frpc (`make download-frp`), python3, curl and bun.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
DIR="$(mktemp -d "${TMPDIR:-/tmp}/trssd-frp-smoke.XXXXXX")"
TRSSD_PID=""
HTTP_PID=""
GO="${GO:-go}"
BUN="${BUN:-bun}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"
[[ -x "$HOME/.bun/bin/bun" && "$BUN" = "bun" ]] && BUN="$HOME/.bun/bin/bun"

cleanup() {
  [[ -n "$TRSSD_PID" ]] && kill "$TRSSD_PID" 2>/dev/null || true
  [[ -n "$HTTP_PID" ]] && kill "$HTTP_PID" 2>/dev/null || true
  rm -rf "$DIR"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  echo "--- trssd ---" >&2
  tail -40 "$DIR/trssd.log" 2>/dev/null >&2 || true
  echo "--- frps ---" >&2
  cat "$DIR"/work/*-frps.log 2>/dev/null >&2 || true
  echo "--- frpc ---" >&2
  cat "$DIR"/work/*.log 2>/dev/null >&2 || true
  echo "--- http ---" >&2
  cat "$DIR/http.log" 2>/dev/null >&2 || true
  exit 1
}

[[ -x bin/frps && -x bin/frpc ]] || fail "missing bin/frps or bin/frpc; run make download-frp"
command -v python3 >/dev/null || fail "python3 required"
command -v curl >/dev/null || fail "curl required"
command -v "$BUN" >/dev/null || fail "bun required"

echo "TRSS_FRP_SUPERVISOR_OK" >"$DIR/hello.txt"
python3 -m http.server 18080 --bind 127.0.0.1 --directory "$DIR" >"$DIR/http.log" 2>&1 &
HTTP_PID=$!

CGO_ENABLED=0 "$GO" build -o "$DIR/trssd" ./cmd/trssd
"$DIR/trssd" --socket "$DIR/control.sock" --ssh-config /dev/null \
  --audit-log "$DIR/audit.log" --frps-listen 127.0.0.1:17000 \
  --frpc "$ROOT/bin/frpc" --frps "$ROOT/bin/frps" --frp-config "$DIR/frp.json" \
  >"$DIR/trssd.log" 2>&1 &
TRSSD_PID=$!

for _ in $(seq 1 100); do
  [[ -S "$DIR/control.sock" ]] && break
  sleep 0.1
done
[[ -S "$DIR/control.sock" ]] || fail "supervisor socket did not appear"

TRSS_SOCKET="$DIR/control.sock" TRSS_PROXY_ID_FILE="$DIR/proxy.id" \
  "$BUN" prototype/terminal/scripts/trssd-frp-smoke.ts apply >/dev/null || fail "apply failed"

reachable=0
for _ in $(seq 1 50); do
  if curl -fsS --max-time 1 http://127.0.0.1:18081/hello.txt >"$DIR/out.txt" 2>/dev/null; then
    reachable=1
    break
  fi
  sleep 0.2
done
[[ "$reachable" = 1 ]] || fail "TCP tunnel did not become reachable"
grep -q TRSS_FRP_SUPERVISOR_OK "$DIR/out.txt" || fail "unexpected tunnel payload"

TRSS_SOCKET="$DIR/control.sock" TRSS_PROXY_ID_FILE="$DIR/proxy.id" \
  "$BUN" prototype/terminal/scripts/trssd-frp-smoke.ts stop >/dev/null || fail "stop failed"

sleep 0.5
if curl -fsS --max-time 1 http://127.0.0.1:18081/hello.txt >/dev/null 2>&1; then
  fail "proxy still reachable after stop"
fi

echo "PASS: managed frps + frpc TCP tunnel"
