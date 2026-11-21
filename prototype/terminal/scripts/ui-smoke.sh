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
GO="${GO:-go}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"
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
"$TRSSD" --socket "$SOCKET" --ssh-config "$CONFIG" --audit-log "$SOCKET_DIR/audit.log" 2>>"$LOG" &
TRSSD_PID=$!
for _ in $(seq 1 50); do
  [[ -S "$SOCKET" ]] && break
  sleep 0.1
done
[[ -S "$SOCKET" ]] || fail "trssd did not create $SOCKET"

"$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace.json") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"

assert_contains "Hosts (3)" "UI loads the real host inventory"
send C-k
assert_contains "Comandos" "command palette opens"
assert_contains "Probar ruta" "route probe command listed"
assert_contains "Exportar diagnóstico saneado" "diagnostics export command listed"
send Escape
sleep 0.3
send C-f
assert_contains "FRP · endpoints" "FRP panel opens"
send n
assert_contains "Nuevo proxy FRP" "FRP proxy form opens"
send Escape
sleep 0.2
send Escape
sleep 0.5
assert_contains "Hosts (3)" "FRP panel closes"
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
assert_contains "Cerrar pestaña" "close dialog shows the close intents"
send Enter
assert_not_contains "[2 ● trss-ui-2]" "closing tab 2 removes only that tab"
assert_contains "1 ● trss-ui-1" "tab 1 remains in the tab bar"

send C-1
sleep 0.3
send 'echo UI_AFTER_CLOSE_OK' Enter
assert_contains "UI_AFTER_CLOSE_OK" "remaining session is still interactive"

if command -v tmux >/dev/null; then
  send C-t
  sleep 0.2
  send C-g
  sleep 0.2
  send Enter
  assert_contains "Ctrl+Tab cambiar" "tmux tab is active"
  assert_contains "tmux:on" "tmux mode is enabled in the UI"
  send '[ -n "$TMUX" ] && echo TMUX_ACTIVE' Enter
  assert_contains "TMUX_ACTIVE" "session runs inside remote tmux"

  send C-w
  send Enter
  sleep 0.8
  send C-t
  sleep 0.2
  send Enter
  assert_contains "Ctrl+Tab cambiar" "reopened tmux tab is active"
  assert_contains "tmux:on" "tmux mode remains enabled after closing the tab"
  send 'echo TMUX_REATTACH_OK' Enter
  assert_contains "TMUX_REATTACH_OK" "reopening the host reattaches the tmux session"
else
  echo "SKIP: tmux not available for the reattach check"
fi

send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: F10 closes the UI"
    break
  fi
  sleep 0.2
done
if "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
  fail "F10 did not close the UI"
fi

# Reopen the UI with the same workspace file and verify pending views.
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace.json") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"
assert_contains "Pendiente" "workspace restores pending tabs, not fake live sessions"
assert_contains "trss-ui-1" "restored tabs keep their host alias"
send Enter
assert_contains "Ctrl+Tab cambiar" "a restored tab reconnects on Enter"
send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: restored workspace closes cleanly"
    break
  fi
  sleep 0.2
done
if "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
  fail "restored workspace did not close"
fi

# Real 10-tab overflow check with a clean workspace.
rm -f "$SOCKET_DIR/workspace10.json"
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace10.json") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"
assert_contains "Hosts (3)" "10-tab UI loaded"
sleep 0.5
for index in $(seq 1 10); do
  send C-t
  sleep 0.1
  if [[ "$index" -gt 1 ]]; then
    send Down
  fi
  send Enter
  assert_contains "[$index " "tab $index opened"
done
assert_contains "‹" "tab bar shows a left overflow counter when the last tab is active"
send C-5
assert_contains "[5 " "middle tab can be activated"
assert_contains "›" "tab bar shows a right overflow counter"
send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: 10 tabs close cleanly"
    break
  fi
  sleep 0.2
done
if "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
  fail "10 tabs did not close"
fi

# Abrupt UI crash: trssd keeps the sessions; a restarted UI must attach them.
rm -f "$SOCKET_DIR/workspace-crash.json"
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace-crash.json") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"
assert_contains "Hosts (3)" "crash UI loaded"
send C-t Enter
assert_contains "[1 " "crash tab 1 opened"
send C-t Down Enter
assert_contains "[2 " "crash tab 2 opened"
send 'echo LIVE_BEFORE_CRASH' Enter
assert_contains "LIVE_BEFORE_CRASH" "live session works before the UI crash"
sleep 0.5

"$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
sleep 0.5

"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace-crash.json") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"
assert_contains "Ctrl+Tab cambiar" "restarted UI attaches live supervisor sessions"
assert_not_contains "Pendiente" "live sessions are not shown as pending"
send 'echo LIVE_REATTACH_OK' Enter
assert_contains "LIVE_REATTACH_OK" "attached live session is interactive"
send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: crash recovery closes cleanly"
    break
  fi
  sleep 0.2
done
if "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
  fail "crash recovery did not close"
fi

# Multiple named workspaces with live-session reuse.
rm -rf "$SOCKET_DIR/workspaces"
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_SOCKET=$(printf '%q' "$SOCKET") TRSS_WORKSPACE_DIR=$(printf '%q' "$SOCKET_DIR/workspaces") TRSS_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/trss.ts"
assert_contains "Hosts (3)" "workspace UI loaded"
send Enter
assert_contains "[1 ● trss-ui-1]" "default workspace tab opened"
send C-p
assert_contains "Workspaces" "workspace picker opens"
send n
send lab
send Enter
assert_contains "Hosts (3)" "new workspace starts with an empty view"
send Down
send Enter
assert_contains "[1 ● trss-ui-2]" "second workspace opens its own tab"
send C-p
send Up
send Enter
assert_contains "[1 ● trss-ui-1]" "switching back reuses the default workspace tab"
send C-p
send Down
send Enter
assert_contains "[1 ● trss-ui-2]" "switching to lab reuses its live tab"

echo "== concurrent SSH logins =="
if command -v "$GO" >/dev/null; then
  (cd "$ROOT" && TRSS_TEST_SSH_ALIAS=trss-ui-1 TRSS_TEST_SSH_CONFIG="$CONFIG" \
    CGO_ENABLED=0 "$GO" test -count=1 -run TestSshLoginConcurrent ./tssh) \
    || fail "concurrent SSH logins"
  echo "PASS: concurrent SSH logins"
else
  echo "SKIP: concurrent SSH logins (go not found)"
fi

send F10
for _ in $(seq 1 30); do
  if ! "$TMUX" has-session -t "$SESSION" 2>/dev/null; then
    echo "PASS: named workspaces switch cleanly"
    exit 0
  fi
  sleep 0.2
done
fail "named workspaces did not close"
