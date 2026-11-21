#!/usr/bin/env bash
# End-to-end UI smoke test: trssd + real sshd + OpenTUI over IPC.
#
# Requires: bun, tmux, sshd, ssh-keygen, and bin/trssd (run `make trssd`).
set -euo pipefail

cd "$(dirname "$0")/.."
HERE="$(pwd)"
ROOT="$(cd ../.. && pwd)"
BUN="${BUN:-$HOME/.bun/bin/bun}"
[[ -x "$BUN" ]] || BUN="$(command -v bun)"
TMUX="${TMUX:-tmux}"
TRSSD="${TRSSD:-$ROOT/bin/trssd}"
SSH_DIR="${TRSS_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/trss-sshd-fixture}"
CONFIG="${TMPDIR:-/tmp}/trss-ui-smoke-ssh-config"
SOCKET_DIR="$(mktemp -d "${TMPDIR:-/tmp}/trss-ui-smoke.XXXXXX")"
SOCKET="$SOCKET_DIR/control.sock"
LOG="${TMPDIR:-/tmp}/trss-ui-smoke.log"
SESSION="trss-ui-smoke"
TRSSD_PID=""

cleanup() {
  "$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
  if [[ -n "$TRSSD_PID" ]]; then
    kill "$TRSSD_PID" 2>/dev/null || true
  fi
  "$HERE/scripts/ssh-fixture.sh" stop >/dev/null 2>&1 || true
  rm -f "$CONFIG" "$LOG"
  rm -rf "$SOCKET_DIR"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  "$TMUX" capture-pane -pt "$SESSION" >&2 2>/dev/null || true
  echo "--- ui log ---" >&2
  cat "$LOG" >&2 2>/dev/null || true
  exit 1
}

assert_contains() {
  local pattern="$1" label="$2"
  for _ in $(seq 1 60); do
    if "$TMUX" capture-pane -pt "$SESSION" 2>/dev/null | grep -Fq "$pattern"; then
      echo "PASS: $label"
      return 0
    fi
    sleep 0.2
  done
  fail "$label (missing: $pattern)"
}

assert_not_contains() {
  local pattern="$1" label="$2"
  for _ in $(seq 1 60); do
    if ! "$TMUX" capture-pane -pt "$SESSION" 2>/dev/null | grep -Fq "$pattern"; then
      echo "PASS: $label"
      return 0
    fi
    sleep 0.2
  done
  fail "$label (still present: $pattern)"
}

send() {
  "$TMUX" send-keys -t "$SESSION" "$@"
}

echo "== TRSS UI smoke test =="
[[ -x "$TRSSD" ]] || fail "missing $TRSSD; run make trssd"
"$HERE/scripts/ssh-fixture.sh" start

USER_NAME="$(id -un)"
cat > "$CONFIG" <<EOF
Host trss-ui-1 trss-ui-2 trss-ui-3
  HostName 127.0.0.1
  Port 2222
  User $USER_NAME
  IdentityFile $SSH_DIR/client_ed25519
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
EOF

rm -f "$SOCKET" "$LOG"
"$TRSSD" --socket "$SOCKET" --ssh-config "$CONFIG" 2>>"$LOG" &
TRSSD_PID=$!
for _ in $(seq 1 50); do
  [[ -S "$SOCKET" ]] && break
  sleep 0.1
done
[[ -S "$SOCKET" ]] || fail "trssd did not create $SOCKET"

"$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"

assert_contains "Hosts (3)" "UI loads the real host inventory"
send Enter
assert_contains "[1 ● trss-ui-1]" "tab 1 opens through trssd"
assert_contains "x@x:" "SSH prompt is visible"

send C-t Down Enter
assert_contains "[2 ● trss-ui-2]" "tab 2 opens without closing tab 1"
send C-t Down Down Enter
assert_contains "[3 ● trss-ui-3]" "tab 3 opens"
assert_contains "1 ● trss-ui-1" "tab bar keeps previous sessions"

send C-1
sleep 0.3
send 'echo UI_TAB1_ALIVE; stty size' Enter
assert_contains "UI_TAB1_ALIVE" "tab 1 accepts commands"
assert_contains "34 90" "tab 1 PTY size is 90x34"

"$TMUX" resize-window -t "$SESSION" -x 100 -y 30
sleep 0.8
send 'stty size' Enter
assert_contains "28 70" "resize reaches the SSH PTY through the supervisor"

send C-2
sleep 0.3
send 'echo UI_TAB2_ALIVE' Enter
assert_contains "UI_TAB2_ALIVE" "tab 2 receives input"

send C-1
sleep 0.3
send 'echo UI_TAB1_SURVIVES' Enter
assert_contains "UI_TAB1_SURVIVES" "background tab 1 survived tab 2"

send C-2
sleep 0.3
send C-w
assert_not_contains "[2 ● trss-ui-2]" "closing tab 2 removes only that tab"
assert_contains "1 ● trss-ui-1" "tab 1 remains in the tab bar"

send C-1
sleep 0.3
send 'echo UI_AFTER_CLOSE_OK' Enter
assert_contains "UI_AFTER_CLOSE_OK" "remaining session is still interactive"

send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: F10 closes the UI"
    exit 0
  fi
  sleep 0.2
done
fail "F10 did not close the UI"
