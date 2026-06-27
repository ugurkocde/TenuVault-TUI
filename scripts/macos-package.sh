#!/usr/bin/env bash
# Build a signed (and optionally notarized) macOS .pkg installer and .dmg for a
# single architecture from an already-signed tenuvault binary.
#
# GoReleaser OSS has no DMG/PKG support, so we use Apple's native tools.
# Requires macOS with the Developer ID Application + Developer ID Installer certs
# available to codesign/productsign (a keychain), and xcrun notarytool.
#
# Usage:
#   scripts/macos-package.sh <version> <arch> <path-to-signed-binary>
#
# Environment:
#   APP_IDENTITY        codesign identity, e.g. "Developer ID Application: Ugur Koc (D259ULY2B4)"
#   INSTALLER_IDENTITY  productsign identity, e.g. "Developer ID Installer: Ugur Koc (D259ULY2B4)"
#   NOTARY_PROFILE      notarytool keychain profile name (skip notarization if empty)
#   KEYCHAIN            optional keychain path to pass to codesign/productsign
#   OUTDIR              output directory (default: ./dist-macos)
set -euo pipefail

VERSION="${1:?version required}"
ARCH="${2:?arch required}"
BIN="${3:?path to signed binary required}"

APP_IDENTITY="${APP_IDENTITY:?APP_IDENTITY required}"
INSTALLER_IDENTITY="${INSTALLER_IDENTITY:?INSTALLER_IDENTITY required}"
NOTARY_PROFILE="${NOTARY_PROFILE:-}"
OUTDIR="${OUTDIR:-dist-macos}"
IDENTIFIER="com.ugurkocde.tenuvault"
KC_ARGS=()
[ -n "${KEYCHAIN:-}" ] && KC_ARGS=(--keychain "$KEYCHAIN")

mkdir -p "$OUTDIR"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PKG="$OUTDIR/tenuvault_${VERSION}_darwin_${ARCH}.pkg"
DMG="$OUTDIR/tenuvault_${VERSION}_darwin_${ARCH}.dmg"

notarize() {
  local artifact="$1"
  if [ -z "$NOTARY_PROFILE" ]; then
    echo "  (no NOTARY_PROFILE — skipping notarization of $artifact)"
    return
  fi
  echo "  notarizing $artifact"
  xcrun notarytool submit "$artifact" --keychain-profile "$NOTARY_PROFILE" --wait
  xcrun stapler staple "$artifact"
}

echo "== PKG =="
PKGROOT="$WORK/pkgroot"
mkdir -p "$PKGROOT"
cp "$BIN" "$PKGROOT/tenuvault"
chmod 755 "$PKGROOT/tenuvault"
pkgbuild --root "$PKGROOT" --identifier "$IDENTIFIER" --version "$VERSION" \
  --install-location /usr/local/bin "$WORK/unsigned.pkg"
productsign ${KC_ARGS[@]+"${KC_ARGS[@]}"} --sign "$INSTALLER_IDENTITY" "$WORK/unsigned.pkg" "$PKG"
pkgutil --check-signature "$PKG" | sed -n '1,3p'
notarize "$PKG"
echo "  -> $PKG"

echo "== DMG =="
DMGSTAGE="$WORK/dmgstage"
mkdir -p "$DMGSTAGE"
cp "$BIN" "$DMGSTAGE/tenuvault"
chmod 755 "$DMGSTAGE/tenuvault"
cat > "$DMGSTAGE/README.txt" <<EOF
TenuVault TUI ${VERSION}

Move 'tenuvault' to a directory on your PATH, e.g.:
  sudo mv tenuvault /usr/local/bin/

Or install via the .pkg, or: brew install ugurkocde/UgurLabs/tenuvault
EOF
hdiutil create -volname "TenuVault ${VERSION}" -srcfolder "$DMGSTAGE" \
  -ov -format UDZO "$WORK/tenuvault.dmg" >/dev/null
codesign ${KC_ARGS[@]+"${KC_ARGS[@]}"} --sign "$APP_IDENTITY" --timestamp "$WORK/tenuvault.dmg"
cp "$WORK/tenuvault.dmg" "$DMG"
codesign --verify --verbose=1 "$DMG" 2>&1 | sed -n '1,2p'
notarize "$DMG"
echo "  -> $DMG"
