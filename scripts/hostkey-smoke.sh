#!/usr/bin/env bash
# Phase 1 P0: strict SSH host key verification.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
TSSH="${TSSH:-$ROOT/bin/tssh}"
DIR="$(mktemp -d "${TMPDIR:-/tmp}/trss-hostkey-smoke.XXXXXX")"
chmod 700 "$DIR"
cleanup() {
  "$ROOT/prototype/terminal/scripts/ssh-fixture.sh" stop >/dev/null 2>&1 || true
  rm -rf "$DIR"
}
trap cleanup EXIT

[[ -x "$TSSH" ]] || { echo "missing $TSSH; run make all" >&2; exit 1; }
command -v ssh-keyscan >/dev/null || { echo "ssh-keyscan required" >&2; exit 1; }
command -v ssh-keygen >/dev/null || { echo "ssh-keygen required" >&2; exit 1; }

"$ROOT/prototype/terminal/scripts/ssh-fixture.sh" start >/dev/null
USER_NAME="$(id -un)"
SSH_DIR="${TRSS_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/trss-sshd-fixture}"
CLIENT_KEY="$SSH_DIR/client_ed25519"

cat > "$DIR/ssh_config" <<EOF
Host trss-hostkey
  HostName 127.0.0.1
  Port 2222
  User $USER_NAME
  IdentityFile $CLIENT_KEY
  UserKnownHostsFile $DIR/known_hosts
  GlobalKnownHostsFile /dev/null
  StrictHostKeyChecking yes
  BatchMode yes
  LogLevel ERROR
EOF

ssh-keyscan -p 2222 127.0.0.1 2>/dev/null > "$DIR/known_hosts"
chmod 600 "$DIR/known_hosts"

if ! "$TSSH" -F "$DIR/ssh_config" trss-hostkey true >"$DIR/correct.out" 2>&1; then
  echo "FAIL: correct host key was rejected" >&2
  cat "$DIR/correct.out" >&2
  exit 1
fi
echo "PASS: correct host key connects"

ssh-keygen -q -t ed25519 -N '' -f "$DIR/wrong_host" >/dev/null
WRONG_PUB="$(ssh-keygen -y -f "$DIR/wrong_host")"
printf '[127.0.0.1]:2222 %s\n' "$WRONG_PUB" > "$DIR/known_hosts"
chmod 600 "$DIR/known_hosts"

set +e
"$TSSH" -F "$DIR/ssh_config" trss-hostkey true >"$DIR/changed.out" 2>&1
STATUS=$?
set -e
if [[ $STATUS -eq 0 ]]; then
  echo "FAIL: changed host key was accepted" >&2
  cat "$DIR/changed.out" >&2
  exit 1
fi
if ! grep -q "REMOTE HOST IDENTIFICATION HAS CHANGED" "$DIR/changed.out"; then
  echo "FAIL: changed host key did not produce the strict warning" >&2
  cat "$DIR/changed.out" >&2
  exit 1
fi
echo "PASS: changed host key is blocked"
