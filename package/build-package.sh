#!/usr/bin/env bash
# build-package.sh — produces the Unraid plugin payload as a Slackware .txz.
#
# Output: dist/engram-deployer-<version>-x86_64-1.txz
#
# Layout inside the .txz (Slackware convention):
#   usr/local/sbin/engram-deployer                  (Go binary)
#   etc/rc.d/rc.engram-deployer                     (rc init)
#   boot/config/plugins/engram-deployer/engram-deployer.env.sample
#   install/slack-desc                              (package metadata)
#   install/doinst.sh                               (post-install hook)
#
# Installed via `installpkg` / `upgradepkg --install-new` on Unraid. The
# Slackware tooling handles extraction with root:root ownership, registers
# files for clean removal via `removepkg`, and runs install/doinst.sh.
#
# Usage: package/build-package.sh <version>
set -euo pipefail

VERSION="${1:?Usage: build-package.sh <version>}"
ARCH="x86_64"
BUILD="1"
PKG_NAME="engram-deployer"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="${ROOT}/dist/stage"
OUT="${ROOT}/dist/${PKG_NAME}-${VERSION}-${ARCH}-${BUILD}.txz"

rm -rf "$STAGE"
mkdir -p \
  "${STAGE}/usr/local/sbin" \
  "${STAGE}/etc/rc.d" \
  "${STAGE}/boot/config/plugins/engram-deployer" \
  "${STAGE}/install"

echo "==> Building Go binary (linux/amd64, static)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags="-s -w -X main.version=${VERSION}" \
  -o "${STAGE}/usr/local/sbin/engram-deployer" \
  "${ROOT}/cmd/deployer"

echo "==> Staging rc.d script + env sample..."
install -m 0755 "${ROOT}/package/rc.engram-deployer" "${STAGE}/etc/rc.d/"
install -m 0644 "${ROOT}/package/engram-deployer.env.sample" \
  "${STAGE}/boot/config/plugins/engram-deployer/"

echo "==> Staging Slackware metadata..."
install -m 0644 "${ROOT}/package/slack-desc" "${STAGE}/install/slack-desc"
install -m 0755 "${ROOT}/package/doinst.sh" "${STAGE}/install/doinst.sh"

echo "==> Packing .txz..."
mkdir -p "$(dirname "$OUT")"
# Force uid=0/gid=0 on every entry. `installpkg` itself normalizes ownership
# during extraction, but a tarball with non-root entries trips audit/CI and
# would chown `/` itself if anyone ever bypassed installpkg with a raw
# `tar -xJf -C /` (which is exactly how the v0.1.2 install bug happened).
tar --owner=0 --group=0 --numeric-owner -cJf "$OUT" -C "$STAGE" .

MD5="$(md5sum "$OUT" | awk '{print $1}')"
SIZE="$(stat -c %s "$OUT")"
echo ""
echo "==> Output: $OUT"
echo "    Size:   ${SIZE} bytes"
echo "    MD5:    ${MD5}"
