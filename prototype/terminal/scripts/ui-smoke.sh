#!/usr/bin/env bash
# End-to-end UI smoke test: ssh05d + real sshd + OpenTUI over IPC.
#
# Requires: bun, tmux, sshd, ssh-keygen, and bin/ssh05d (run `make ssh05d`).
set -euo pipefail

cd "$(dirname "$0")/.."
HERE="$(pwd)"
ROOT="$(cd ../.. && pwd)"
BUN="${BUN:-$HOME/.bun/bin/bun}"
[[ -x "$BUN" ]] || BUN="$(command -v bun)"
GO="${GO:-go}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"
TMUX="${TMUX:-tmux}"
SSH05D="${SSH05D:-$ROOT/bin/ssh05d}"
SSH_DIR="${SSH05_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/ssh05-sshd-fixture}"
CONFIG="${TMPDIR:-/tmp}/ssh05-ui-smoke-ssh-config"
SOCKET_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ssh05-ui-smoke.XXXXXX")"
SOCKET="$SOCKET_DIR/control.sock"
LOG="${TMPDIR:-/tmp}/ssh05-ui-smoke.log"
SESSION="ssh05-ui-smoke"
SSH05D_PID=""

cleanup() {
  "$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
  if [[ -n "$SSH05D_PID" ]]; then
    kill "$SSH05D_PID" 2>/dev/null || true
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

echo "== ssh05 UI smoke test =="
[[ -x "$SSH05D" ]] || fail "missing $SSH05D; run make ssh05d"
"$HERE/scripts/ssh-fixture.sh" start

USER_NAME="$(id -un)"
cat > "$CONFIG" <<EOF
Host ssh05-ui-1 ssh05-ui-2 ssh05-ui-3
  HostName 127.0.0.1
  Port 2222
  User $USER_NAME
  IdentityFile $SSH_DIR/client_ed25519
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
EOF

rm -f "$SOCKET" "$LOG"
TSSH_DISABLE_DOCKER=1 SSH05_HOST_OVERRIDES="$SOCKET_DIR/hosts.json" \
  "$SSH05D" --socket "$SOCKET" --ssh-config "$CONFIG" --audit-log "$SOCKET_DIR/audit.log" 2>>"$LOG" &
SSH05D_PID=$!
for _ in $(seq 1 50); do
  [[ -S "$SOCKET" ]] && break
  sleep 0.1
done
[[ -S "$SOCKET" ]] || fail "ssh05d did not create $SOCKET"

"$TMUX" kill-session -t "$SESSION" 2>/dev/null || true
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace.json") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"

assert_contains "Bienvenido" "welcome message appears"
send Enter
sleep 0.3
send Down
sleep 0.3
assert_contains "Hosts SSH (3)" "hosts are grouped by category"
assert_contains "Hosts (3)" "UI loads the real host inventory"
send Right
assert_contains "Conectar" "right arrow opens the host actions menu"
send Escape
sleep 0.2
send Left
send Enter
assert_contains "Menú" "left arrow focuses the action bar"
send Escape
sleep 0.3
send /
send c
send o
send n
send n
send Enter
assert_contains "Nuevo host" "slash command /conn opens a new host"
send "vuigram server"
send Tab
send "127.0.0.1"
send C-s
sleep 0.6
assert_contains "vuigram-server" "alias with spaces is normalized"
send vuigram
assert_contains "vuigram-server" "new host can be filtered"
send Right
assert_contains "Conectar" "right arrow opens the host actions menu"
assert_contains "Editar" "host actions menu offers edit"
assert_contains "Eliminar" "host actions menu offers delete"
send Down
send Enter
assert_contains "Editar host" "edit action opens the host form"
send Home
send "z-"
send C-s
sleep 0.6
assert_contains "z-vuigram-server" "alias rename and middle-of-field edit work"
send Right
send Down
send Down
send Enter
assert_contains "Eliminar host" "delete action asks for confirmation"
send Enter
sleep 0.6
assert_contains "Sin resultados" "delete removes the host"
send Escape
sleep 0.3
assert_contains "Menú" "action bar is visible"
send Tab
send Enter
assert_contains "Menú · 1/" "actions menu opens from the button bar"
send Escape
sleep 0.3
send F2
assert_contains "★ssh05-ui-1" "favorite metadata is persisted"
send F4
send prod
send Enter
assert_contains "(prod)" "label metadata is persisted"
send C-k
assert_contains "Comandos" "command palette opens"
assert_contains "Probar ruta" "route probe command listed"
assert_contains "Exportar diagnóstico saneado" "diagnostics export command listed"
assert_contains "Diagnosticar host" "host diagnostics command listed"
assert_contains "Preferencias" "preferences command listed"
assert_contains "Transferencias" "transfers command listed"
assert_contains "Escanear red local (Nmap)" "nmap command listed"
assert_contains "Información de red" "network info command listed"
assert_contains "Ayuda" "help command listed"
assert_contains "Nuevo host" "new host command listed"
send Escape
sleep 0.3
PASTE_SEQ=$(printf '\033[200~ssh05-ui-2\033[201~')
"$TMUX" send-keys -t "$SESSION" -l -- "$PASTE_SEQ"
assert_contains "/ssh05-ui-2" "bracketed paste fills the host search"
send Escape
sleep 0.3
send F1
assert_contains "Ayuda · atajos" "help overlay opens"
send Escape
sleep 0.3
send F6
send Enter
assert_contains "Menú · 1/" "actions menu opens from the action bar with Enter"
send Down
send Enter
assert_contains "Nuevo host" "submenu opens with arrow selection"
assert_contains "Contraseña:" "new host form offers an encoded password field"
send Right
send Right
send Left
send Up
send Escape
sleep 0.3
assert_contains "Hosts (3)" "new host form closes with Esc"
send F6
send Right Right Right Right Right Right Right Right
send Enter
assert_contains "Preferencias" "preferences opens through action bar arrows"
send Down
send Right
send Left
send Down
send Left
send Right
send Up
send Up
send Right
send Left
send Enter
sleep 0.3
assert_contains "Hosts (3)" "preferences closes with Enter"
send C-f
assert_contains "FRP · endpoints" "FRP panel opens"
send n
assert_contains "Nuevo proxy FRP" "FRP proxy form opens"
send Down
send Space
assert_contains "Tipo: udp" "space changes the proxy type"
send Right
send Left
send Up
send Space
send Escape
sleep 0.2
send Escape
sleep 0.5
assert_contains "Hosts (3)" "FRP panel closes"
send F6
send Enter
send Down Down Down Down Down Down Down Down Down Down Down Down
send Enter
assert_contains "Transferencias" "transfers panel opens through menu arrows"
send u
assert_contains "Subir archivo" "transfer form opens with u"
send Down Down
send Space
assert_contains "Protocolo: scp" "space changes the transfer protocol"
send Up Up
send Escape
sleep 0.2
send Escape
sleep 0.3
assert_contains "Hosts (3)" "transfers panel closes"
send Enter
assert_contains "[1 ● ssh05-ui-1]" "tab 1 opens through ssh05d"
assert_contains "x@x:" "SSH prompt is visible"

send C-t Down Enter
assert_contains "[2 ● ssh05-ui-2]" "tab 2 opens without closing tab 1"
send C-t Down Down Enter
assert_contains "[3 ● ssh05-ui-3]" "tab 3 opens"
assert_contains "1 ● ssh05-ui-1" "tab bar keeps previous sessions"

send C-1
sleep 0.3
send 'echo UI_TAB1_ALIVE; stty size' Enter
assert_contains "UI_TAB1_ALIVE" "tab 1 accepts commands"
assert_contains "30 88" "tab 1 PTY size is 88x30"

"$TMUX" resize-window -t "$SESSION" -x 100 -y 30
sleep 0.8
send 'stty size' Enter
assert_contains "24 68" "resize reaches the SSH PTY through the supervisor"

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
assert_contains "Cerrar conexión" "close dialog shows the close intents"
send Enter
assert_not_contains "[2 ● ssh05-ui-2]" "closing tab 2 removes only that tab"
assert_contains "1 ● ssh05-ui-1" "tab 1 remains in the tab bar"

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
  "cd $(printf '%q' "$HERE") && SSH05_NO_WELCOME=1 TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace.json") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"
assert_contains "Pendiente" "workspace restores pending tabs, not fake live sessions"
assert_contains "ssh05-ui-1" "restored tabs keep their host alias"
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
  "cd $(printf '%q' "$HERE") && SSH05_NO_WELCOME=1 TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace10.json") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"
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

# Abrupt UI crash: ssh05d keeps the sessions; a restarted UI must attach them.
rm -f "$SOCKET_DIR/workspace-crash.json"
"$TMUX" new-session -d -s "$SESSION" -x 120 -y 36 \
  "cd $(printf '%q' "$HERE") && SSH05_NO_WELCOME=1 TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace-crash.json") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"
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
  "cd $(printf '%q' "$HERE") && SSH05_NO_WELCOME=1 TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE=$(printf '%q' "$SOCKET_DIR/workspace-crash.json") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"
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
  "cd $(printf '%q' "$HERE") && SSH05_NO_WELCOME=1 TERM=xterm-256color SSH05_PREFERENCES=$(printf '%q' "$SOCKET_DIR/preferences.json") SSH05_SOCKET=$(printf '%q' "$SOCKET") SSH05_WORKSPACE_DIR=$(printf '%q' "$SOCKET_DIR/workspaces") SSH05_UI_DEBUG=$(printf '%q' "$LOG") $(printf '%q' "$BUN") run src/ssh05.ts"
assert_contains "Hosts (3)" "workspace UI loaded"
send Enter
assert_contains "[1 ● ssh05-ui-1]" "default workspace tab opened"
send C-p
assert_contains "Workspaces" "workspace picker opens"
send n
send lab
send Enter
assert_contains "Hosts (3)" "new workspace starts with an empty view"
send Down
send Enter
assert_contains "[1 ● ssh05-ui-2]" "second workspace opens its own tab"
send C-p
send Up
send Enter
assert_contains "[1 ● ssh05-ui-1]" "switching back reuses the default workspace tab"
send C-p
send Down
send Enter
assert_contains "[1 ● ssh05-ui-2]" "switching to lab reuses its live tab"

echo "== file transfers =="
SSH05_SOCKET="$SOCKET" SSH05_TEST_SSH_ALIAS=ssh05-ui-1 \
  "$BUN" "$HERE/scripts/transfers-smoke.ts" || fail "file transfers"

echo "== host diagnostics =="
SSH05_SOCKET="$SOCKET" SSH05_TEST_SSH_ALIAS=ssh05-ui-1 \
  "$BUN" "$HERE/scripts/host-diagnostics-smoke.ts" || fail "host diagnostics"

echo "== tmux restore policy =="
SSH05_SOCKET="$SOCKET" SSH05_TEST_SSH_ALIAS=ssh05-ui-1 \
  "$BUN" "$HERE/scripts/tmux-reattach-smoke.ts" || fail "tmux restore policy"
for name in $("$TMUX" list-sessions -F '#S' 2>/dev/null | grep '^ssh05-created-' || true); do
  "$TMUX" kill-session -t "$name" 2>/dev/null || true
done

echo "== concurrent SSH logins =="
if command -v "$GO" >/dev/null; then
  (cd "$ROOT" && SSH05_TEST_SSH_ALIAS=ssh05-ui-1 SSH05_TEST_SSH_CONFIG="$CONFIG" \
    CGO_ENABLED=0 "$GO" test -count=1 -run TestSshLoginConcurrent ./ssh05) \
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
