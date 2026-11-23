#!/usr/bin/env bash
# Build one .deb for ssh05. Usage: build-deb.sh [ubuntu|debian] [amd64|arm64|...]
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
DISTRO="${1:-${DISTRO:-ubuntu}}"
ARCH="${2:-${ARCH:-amd64}}"
VERSION="${VERSION:-0.3.23}"
VERSION="${VERSION#v}"
OUT="${OUT:-$ROOT/dist}"
GO="${GO:-go}"
BUN_VERSION="${BUN_VERSION:-1.4.5}"
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/ssh05-deb"
[[ -x "$HOME/.local/go/bin/go" && "$GO" = "go" ]] && GO="$HOME/.local/go/bin/go"

case "$ARCH" in
  amd64) BUN_ARCH=x64; BUN_CPU=x64 ;;
  arm64) BUN_ARCH=aarch64; BUN_CPU=arm64 ;;
  arm) DEB_ARCH=armhf; BUN_ARCH=armv7l; BUN_CPU=arm ;;
  386) DEB_ARCH=i386; BUN_ARCH=x64; BUN_CPU=ia32 ;;
  *) DEB_ARCH="$ARCH"; BUN_ARCH="$ARCH"; BUN_CPU="$ARCH" ;;
esac
DEB_ARCH="${DEB_ARCH:-$ARCH}"

case "$DISTRO" in
  ubuntu|debian) ;;
  *) echo "unsupported distro label: $DISTRO (use ubuntu or debian)" >&2; exit 2 ;;
esac

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/ssh05-deb.XXXXXX")"
BUILD="$(mktemp -d "${TMPDIR:-/tmp}/ssh05-ui.XXXXXX")"
cleanup() { rm -rf "$STAGE" "$BUILD"; }
trap cleanup EXIT

host_bun() {
  if [[ -n "${BUN:-}" && -x "$BUN" ]]; then echo "$BUN"; return; fi
  if command -v bun >/dev/null 2>&1; then command -v bun; return; fi
  if [[ -x "$HOME/.bun/bin/bun" ]]; then echo "$HOME/.bun/bin/bun"; return; fi
  local harch
  case "$(uname -m)" in
    x86_64|amd64) harch=x64 ;;
    aarch64|arm64) harch=aarch64 ;;
    *) return 1 ;;
  esac
  bun_binary_for_arch "$harch"
}

# bun_binary_for_arch returns a runner for the given Bun arch, caching downloads.
bun_binary_for_arch() {
  local bun_arch="$1"
  local target="$CACHE/bun-linux-$bun_arch-$BUN_VERSION"
  if [[ -x "$target" ]]; then echo "$target"; return; fi
  mkdir -p "$CACHE"
  local zip="$CACHE/bun-linux-$bun_arch-$BUN_VERSION.zip"
  if [[ ! -f "$zip" ]]; then
    echo "downloading bun $BUN_VERSION for $bun_arch..." >&2
    curl -fsSL -o "$zip" \
      "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-${bun_arch}.zip"
  fi
  local unpack="$CACHE/unpack-$bun_arch"
  rm -rf "$unpack" && mkdir -p "$unpack"
  unzip -q -o "$zip" -d "$unpack"
  local found
  found="$(find "$unpack" -type f -name bun | head -n1)"
  [[ -n "$found" ]] || { echo "bun binary missing in $zip" >&2; exit 1; }
  install -m 0755 "$found" "$target"
  echo "$target"
}

bun_for_arch() { bun_binary_for_arch "$BUN_ARCH"; }

# node_modules for the target architecture, cached by lockfile hash.
ui_modules() {
  local marker="$CACHE/ui-$ARCH"
  local lockhash
  lockhash="$(sha256sum "$ROOT/prototype/terminal/package.json" "$ROOT/prototype/terminal/bun.lock" | sha256sum | cut -d' ' -f1)"
  if [[ -d "$marker/node_modules" && -f "$marker/hash" && "$(cat "$marker/hash")" = "$lockhash" ]]; then
    echo "$marker/node_modules"; return
  fi
  local host
  host="$(host_bun)" || {
    echo "bun is required to install the UI dependencies" >&2
    exit 1
  }
  rm -rf "$marker" && mkdir -p "$marker"
  cp "$ROOT/prototype/terminal/package.json" "$ROOT/prototype/terminal/bun.lock" "$marker/"
  (cd "$marker" && "$host" install --production --frozen-lockfile --os=linux --cpu="$BUN_CPU" >/dev/null)
  printf '%s' "$lockhash" > "$marker/hash"
  echo "$marker/node_modules"
}

mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/bin" "$STAGE/usr/share/doc/ssh05" \
  "$STAGE/usr/lib/systemd/user" "$STAGE/etc/systemd/user/default.target.wants"
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" "$GO" build -trimpath -ldflags "-s -w" \
  -o "$STAGE/usr/bin/ssh05" ./cmd/ssh05
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" "$GO" build -trimpath -ldflags "-s -w" \
  -o "$STAGE/usr/bin/ssh05d" ./cmd/ssh05d
chmod 0755 "$STAGE/usr/bin/ssh05" "$STAGE/usr/bin/ssh05d"

# Supervisor unit plus global user enablement, so it starts at login without
# anyone running systemctl.
cp packaging/ssh05d.service "$STAGE/usr/lib/systemd/user/ssh05d.service"
chmod 0644 "$STAGE/usr/lib/systemd/user/ssh05d.service"
ln -s /usr/lib/systemd/user/ssh05d.service \
  "$STAGE/etc/systemd/user/default.target.wants/ssh05d.service"

# OpenTUI frontend with its pinned Bun runtime and production dependencies.
UI_ROOT="$STAGE/usr/lib/ssh05"
UI_APP="$UI_ROOT/prototype/terminal"
mkdir -p "$UI_APP" "$UI_ROOT/contracts"
cp -a "$ROOT/prototype/terminal/src" "$UI_APP/src"
cp "$ROOT/prototype/terminal/package.json" "$UI_APP/package.json"
cp -a "$ROOT/contracts/ts" "$UI_ROOT/contracts/ts"
cp -a "$(ui_modules)" "$UI_APP/node_modules"
install -m 0755 "$(bun_for_arch)" "$UI_ROOT/bun"
cat > "$STAGE/usr/bin/ssh05-ui" <<'EOF'
#!/bin/sh
# ssh05 OpenTUI frontend shipped in the package.
exec /usr/lib/ssh05/bun run /usr/lib/ssh05/prototype/terminal/src/ssh05.ts "$@"
EOF
chmod 0755 "$STAGE/usr/bin/ssh05-ui"

cp README.md "$STAGE/usr/share/doc/ssh05/README.md"
cp debian/copyright "$STAGE/usr/share/doc/ssh05/copyright"
cp packaging/THIRD-PARTY.md "$STAGE/usr/share/doc/ssh05/THIRD-PARTY.md"
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
Recommends: tmux, zenity
Homepage: https://github.com/lordpietre/ssh05
Description: OpenSSH-compatible client with TUI, supervisor and OpenTUI frontend
 ssh05 is a highly OpenSSH-compatible client with a TUI host manager,
 optional tmux sessions, FRP tunnels, diagnostics and SCP/SFTP transfers.
 It includes ssh05d (the local supervisor, enabled as a systemd user unit),
 ssh05-ui (the OpenTUI frontend with its pinned Bun runtime) and the trzsz
 and Zmodem filters for interactive sessions.
 Soporte: dinamicas.electronicas@gmail.com
 This package is built for ${DISTRO} ${DEB_ARCH}.
EOF
chmod 0644 "$STAGE/DEBIAN/control"

mkdir -p "$OUT"
DEB="$OUT/ssh05_${VERSION}_${DISTRO}_${DEB_ARCH}.deb"
dpkg-deb --root-owner-group --build "$STAGE" "$DEB" >/dev/null
(cd "$OUT" && sha256sum "$(basename "$DEB")" > "$(basename "$DEB").sha256")
echo "built $DEB"
