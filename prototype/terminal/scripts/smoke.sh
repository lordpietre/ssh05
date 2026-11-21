#!/usr/bin/env bash
# Phase 0 acceptance smoke test for the OpenTUI + Bun PTY prototype.
#
# Requirements: bun >= 1.4.5, tmux, vim, htop.
# The SSH case starts a throwaway sshd on 127.0.0.1:2222 unless
# TRSS_PROTO_SKIP_SSH=1 is set.
set -euo pipefail

cd "$(dirname "$0")/.."
HERE="$(pwd)"
BUN="${BUN:-$HOME/.bun/bin/bun}"
[[ -x "$BUN" ]] || BUN="$(command -v bun)"
TMUX="${TMUX:-tmux}"
SSH_PORT="${TRSS_SSH_PORT:-2222}"
SSH_DIR="${TRSS_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/trss-sshd-fixture}"
DEBUG_LOG="${TRSS_PROTO_LOG:-${TMPDIR:-/tmp}/trss-proto-smoke.log}"
SESSIONS=()

cleanup() {
  for session in "${SESSIONS[@]:-}"; do
    "$TMUX" kill-session -t "$session" 2>/dev/null || true
  done
  if [[ "${TRSS_PROTO_SKIP_SSH:-0}" != "1" ]]; then
    "$HERE/scripts/ssh-fixture.sh" stop >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local session="$1" pattern="$2" label="$3"
  for _ in $(seq 1 60); do
    if "$TMUX" capture-pane -pt "$session" 2>/dev/null | grep -Fq "$pattern"; then
      echo "PASS: $label"
      return 0
    fi
    sleep 0.2
  done
  echo "--- captured pane ($session) ---" >&2
  "$TMUX" capture-pane -pt "$session" >&2 2>/dev/null || true
  fail "$label (missing: $pattern)"
}

start_app() {
  local session="$1" width="$2" height="$3"
  shift 3
  local cmd
  cmd="$(printf '%q ' "$@")"
  SESSIONS+=("$session")
  rm -f "$DEBUG_LOG"
  "$TMUX" kill-session -t "$session" 2>/dev/null || true
  "$TMUX" new-session -d -s "$session" -x "$width" -y "$height" \
    "cd $(printf '%q' "$HERE") && TERM=xterm-256color TRSS_PROTO_DEBUG=$(printf '%q' "$DEBUG_LOG") $(printf '%q' "$BUN") run src/main.ts -- $cmd"
  assert_contains "$session" "TRSS · Phase 0 prototype" "TUI started ($session)"
}

resize_app() {
  local session="$1" width="$2" height="$3"
  "$TMUX" resize-window -t "$session" -x "$width" -y "$height"
  # Give OpenTUI a frame to apply the new layout and the PTY resize to land.
  sleep 0.6
}

test_resize() {
  start_app trss-smoke-size 120 36 ./scripts/echo-size.sh
  assert_contains trss-smoke-size "UNICODE ñ 漢 🚀" "Unicode output renders in the embedded terminal"
  assert_contains trss-smoke-size "READY 34 120" "initial PTY size is 120x34"
  resize_app trss-smoke-size 100 30
  assert_contains trss-smoke-size "RESIZED 28 100" "resize propagates to the child PTY (100x28)"
}

test_vim() {
  rm -f /tmp/trss-proto-vim.txt /tmp/.trss-proto-vim.txt.swp
  start_app trss-smoke-vim 120 36 vim -u NONE -N /tmp/trss-proto-vim.txt
  "$TMUX" send-keys -t trss-smoke-vim 'iVIM_INTERACTIVE_OK'
  "$TMUX" send-keys -t trss-smoke-vim Escape
  assert_contains trss-smoke-vim "VIM_INTERACTIVE_OK" "vim receives input and renders in the embedded terminal"
  resize_app trss-smoke-vim 100 30
  assert_contains trss-smoke-vim "VIM_INTERACTIVE_OK" "vim survives an alt-screen resize"
}

test_htop() {
  start_app trss-smoke-htop 120 36 htop
  assert_contains trss-smoke-htop "Tasks:" "htop renders (alt-screen + live updates)"
  assert_contains trss-smoke-htop "Load average" "htop status line is visible"
  "$TMUX" send-keys -t trss-smoke-htop q
}

test_keys() {
  start_app trss-smoke-keys 120 36 cat
  "$TMUX" send-keys -t trss-smoke-keys 'HELLO_KEYS' Enter
  assert_contains trss-smoke-keys "HELLO_KEYS" "keyboard input reaches the child PTY"
  "$TMUX" send-keys -t trss-smoke-keys '/'
  "$TMUX" send-keys -t trss-smoke-keys Escape
  sleep 0.3
  "$TMUX" has-session -t trss-smoke-keys 2>/dev/null || fail "Escape must not close the session"
  echo "PASS: Escape and / do not close the TUI"
  "$TMUX" send-keys -t trss-smoke-keys F10
  sleep 0.5
  if "$TMUX" has-session -t trss-smoke-keys 2>/dev/null; then
    fail "F10 did not close the TUI"
  fi
  echo "PASS: F10 closes the TUI"
}

test_tmux() {
  local socket="trss-proto-$$"
  start_app trss-smoke-tmux 120 36 tmux -f /dev/null -L "$socket" new-session -A -s nested \
    "printf 'TMUX_NESTED_OK\\n'; sleep 3"
  assert_contains trss-smoke-tmux "TMUX_NESTED_OK" "tmux runs inside the embedded terminal"
  tmux -L "$socket" kill-server 2>/dev/null || true
}

test_ssh() {
  if [[ "${TRSS_PROTO_SKIP_SSH:-0}" == "1" ]]; then
    echo "SKIP: SSH case disabled by TRSS_PROTO_SKIP_SSH=1"
    return 0
  fi
  if ! command -v sshd >/dev/null || ! command -v ssh-keygen >/dev/null; then
    echo "SKIP: sshd/ssh-keygen not available"
    return 0
  fi
  "$HERE/scripts/ssh-fixture.sh" start
  local user
  user="$(id -un)"
  start_app trss-smoke-ssh 120 36 \
    ssh -i "$SSH_DIR/client_ed25519" -p "$SSH_PORT" \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "$user@127.0.0.1"
  assert_contains trss-smoke-ssh "$user@" "SSH shell prompt is visible"
  "$TMUX" send-keys -t trss-smoke-ssh 'echo SSH_CMD_OK; stty size' Enter
  assert_contains trss-smoke-ssh "SSH_CMD_OK" "commands run in the real SSH PTY"
  assert_contains trss-smoke-ssh "34 120" "SSH PTY starts at 120x34"
  resize_app trss-smoke-ssh 100 30
  "$TMUX" send-keys -t trss-smoke-ssh 'stty size' Enter
  assert_contains trss-smoke-ssh "28 100" "SSH window-change reaches the remote PTY"
}

echo "== TRSS Phase 0 prototype smoke test =="
echo "bun:  $($BUN --version)"
echo "dir:  $HERE"
test_resize
test_vim
test_htop
test_keys
test_tmux
test_ssh
echo "== all checks passed =="
