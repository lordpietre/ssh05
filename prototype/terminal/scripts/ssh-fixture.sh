#!/usr/bin/env bash
# Starts/stops a throwaway OpenSSH server for the Phase 0 prototype tests.
# It runs as the current user on a high port and writes everything under
# ${TMPDIR:-/tmp}/ssh05-sshd-fixture. No sudo is required.
set -euo pipefail

ACTION="${1:-start}"
DIR="${SSH05_SSH_FIXTURE_DIR:-${TMPDIR:-/tmp}/ssh05-sshd-fixture}"
PORT="${SSH05_SSH_PORT:-2222}"
USER_NAME="$(id -un)"

stop_server() {
  if [[ -f "$DIR/sshd.pid" ]]; then
    kill "$(cat "$DIR/sshd.pid")" 2>/dev/null || true
    rm -f "$DIR/sshd.pid"
  fi
}

case "$ACTION" in
  start)
    command -v sshd >/dev/null || { echo "sshd not found" >&2; exit 1; }
    command -v ssh-keygen >/dev/null || { echo "ssh-keygen not found" >&2; exit 1; }
    SSHD="$(command -v sshd)"
    stop_server
    mkdir -p "$DIR"
    if [[ ! -f "$DIR/host_ed25519" ]]; then
      ssh-keygen -q -t ed25519 -N '' -f "$DIR/host_ed25519"
    fi
    if [[ ! -f "$DIR/client_ed25519" ]]; then
      ssh-keygen -q -t ed25519 -N '' -f "$DIR/client_ed25519"
    fi
    cp "$DIR/client_ed25519.pub" "$DIR/authorized_keys"
    chmod 600 "$DIR/authorized_keys" "$DIR/client_ed25519" "$DIR/host_ed25519"
    cat > "$DIR/sshd_config" <<EOF
Port $PORT
ListenAddress 127.0.0.1
HostKey $DIR/host_ed25519
PidFile $DIR/sshd.pid
AuthorizedKeysFile $DIR/authorized_keys
StrictModes no
UsePAM no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
AllowUsers $USER_NAME
LogLevel VERBOSE
Subsystem sftp internal-sftp
EOF
    if [[ -n "${SSH05_SSH_FIXTURE_PATH:-}" ]]; then
      printf 'SetEnv PATH="%s"\n' "$SSH05_SSH_FIXTURE_PATH" >> "$DIR/sshd_config"
    fi
    "$SSHD" -t -f "$DIR/sshd_config"
    "$SSHD" -f "$DIR/sshd_config" -E "$DIR/sshd.log"
    for _ in $(seq 1 50); do
      if ssh -i "$DIR/client_ed25519" -p "$PORT" \
        -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=1 "$USER_NAME@127.0.0.1" true >/dev/null 2>&1; then
        echo "sshd ready on 127.0.0.1:$PORT"
        echo "key: $DIR/client_ed25519"
        exit 0
      fi
      sleep 0.1
    done
    echo "sshd did not become ready; see $DIR/sshd.log" >&2
    exit 1
    ;;
  stop)
    stop_server
    echo "sshd stopped"
    ;;
  *)
    echo "usage: $0 start|stop" >&2
    exit 2
    ;;
esac
