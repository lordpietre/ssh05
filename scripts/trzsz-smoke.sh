#!/usr/bin/env bash
# DAT-01: real trzsz (tsz) download through the supervisor session filter.
#
# Builds the trzsz tools from the pinned Go module (no network) and runs the
# opt-in integration test against the local sshd fixture.
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
DIR="${SSH05_TRZSZ_SMOKE_DIR:-${TMPDIR:-/tmp}/ssh05-trzsz-smoke}"
PORT="${SSH05_SSH_PORT:-2222}"
BIN="$DIR/bin"
DOWNLOAD="$DIR/download"
CONFIG="$DIR/ssh-config"
GO="${GO:-go}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"

cleanup() {
  ./prototype/terminal/scripts/ssh-fixture.sh stop >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$BIN" "$DOWNLOAD"
if [[ ! -x "$BIN/tsz" || ! -x "$BIN/trz" ]]; then
  echo "building trzsz tools from the pinned module..."
  CGO_ENABLED=0 "$GO" build -o "$BIN/tsz" github.com/trzsz/trzsz-go/cmd/tsz
  CGO_ENABLED=0 "$GO" build -o "$BIN/trz" github.com/trzsz/trzsz-go/cmd/trz
fi

USER_NAME="$(id -un)"
cat > "$CONFIG" <<EOF
Host ssh05-trzsz
  HostName 127.0.0.1
  Port $PORT
  User $USER_NAME
  IdentityFile ${SSH05_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/ssh05-sshd-fixture}/client_ed25519
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
EOF

export SSH05_SSH_FIXTURE_PATH="$BIN:/usr/local/bin:/usr/bin:/bin"
./prototype/terminal/scripts/ssh-fixture.sh start

SSH05_TEST_SSH_ALIAS=ssh05-trzsz \
SSH05_TEST_SSH_CONFIG="$CONFIG" \
SSH05_TEST_TRZSZ_DOWNLOAD="$DOWNLOAD" \
  CGO_ENABLED=0 "$GO" test -count=1 -run TestSessionTrzszIntegration -v ./internal/supervisor

echo "PASS: trzsz download through the supervisor session filter"
