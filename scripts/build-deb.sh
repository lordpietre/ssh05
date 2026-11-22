#!/usr/bin/env bash
# Build one .deb for ssh05. Usage: build-deb.sh [ubuntu|debian] [amd64|arm64|...]
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
DISTRO="${1:-${DISTRO:-ubuntu}}"
ARCH="${2:-${ARCH:-amd64}}"
VERSION="${VERSION:-0.3.18}"
VERSION="${VERSION#v}"
OUT="${OUT:-$ROOT/dist}"
GO="${GO:-go}"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"

case "$ARCH" in
  arm) DEB_ARCH=armhf ;;
  386) DEB_ARCH=i386 ;;
  *) DEB_ARCH="$ARCH" ;;
esac

case "$DISTRO" in
  ubuntu|debian) ;;
  *) echo "unsupported distro label: $DISTRO (use ubuntu or debian)" >&2; exit 2 ;;
esac

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/ssh05-deb.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/bin" "$STAGE/usr/share/doc/ssh05" \
  "$STAGE/usr/lib/systemd/user"
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" "$GO" build -trimpath -ldflags "-s -w" \
  -o "$STAGE/usr/bin/ssh05" ./cmd/ssh05
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" "$GO" build -trimpath -ldflags "-s -w" \
  -o "$STAGE/usr/bin/ssh05d" ./cmd/ssh05d
chmod 0755 "$STAGE/usr/bin/ssh05" "$STAGE/usr/bin/ssh05d"
cp packaging/ssh05d.service "$STAGE/usr/lib/systemd/user/ssh05d.service"
chmod 0644 "$STAGE/usr/lib/systemd/user/ssh05d.service"
cp README.md "$STAGE/usr/share/doc/ssh05/README.md"
cp debian/copyright "$STAGE/usr/share/doc/ssh05/copyright"
gzip -9n -c "$STAGE/usr/share/doc/ssh05/README.md" > "$STAGE/usr/share/doc/ssh05/README.md.gz"
rm "$STAGE/usr/share/doc/ssh05/README.md"

SIZE="$(du -sk "$STAGE" | cut -f1)"
cat > "$STAGE/DEBIAN/control" <<EOF
Package: ssh05
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${DEB_ARCH}
Maintainer: Dinámicas Electrónicas <dinamicas.electronicas@gmail.com>
Installed-Size: ${SIZE}
Homepage: https://github.com/lordpietre/ssh05
Description: OpenSSH-compatible client with TUI and local supervisor
 ssh05 is a highly OpenSSH-compatible client with a TUI host manager,
 optional tmux sessions, FRP tunnels, diagnostics and SCP/SFTP transfers.
 It also includes ssh05d, the local supervisor used by the OpenTUI frontend.
 Soporte: dinamicas.electronicas@gmail.com
 This package is built for ${DISTRO} ${DEB_ARCH}.
EOF
chmod 0644 "$STAGE/DEBIAN/control"

mkdir -p "$OUT"
DEB="$OUT/ssh05_${VERSION}_${DISTRO}_${DEB_ARCH}.deb"
dpkg-deb --root-owner-group --build "$STAGE" "$DEB" >/dev/null
(cd "$OUT" && sha256sum "$(basename "$DEB")" > "$(basename "$DEB").sha256")
echo "built $DEB"
