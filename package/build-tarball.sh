#!/usr/bin/env bash
# build-tarball.sh — produces the Unraid plugin payload tarball.
#
# Output: dist/engram-deployer-<version>.tar.gz containing:
#   usr/local/sbin/engram-deployer        (Go binary, linux/amd64 static)
#   etc/rc.d/rc.engram-deployer           (start/stop script)
#   boot/config/plugins/engram-deployer/engram-deployer.env.sample
#
# Layout matches `tar -xzf <tarball> -C /`. The plugin's install hook
# extracts it directly to the root filesystem; persistent files (cert,
# env, plugin docs) live under /boot/config/plugins/engram-deployer/.
#
# Usage: package/build-tarball.sh <version>
set -euo pipefail

VERSION="${1:?Usage: build-tarball.sh <version>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="${ROOT}/dist/stage"
OUT="${ROOT}/dist/engram-deployer-${VERSION}.tar.gz"

rm -rf "$STAGE"
mkdir -p \
  "${STAGE}/usr/local/sbin" \
  "${STAGE}/etc/rc.d" \
  "${STAGE}/boot/config/plugins/engram-deployer"

echo "==> Building Go binary (linux/amd64, static)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags="-s -w -X main.version=${VERSION}" \
  -o "${STAGE}/usr/local/sbin/engram-deployer" \
  "${ROOT}/cmd/deployer"

echo "==> Staging rc.d script..."
install -m 0755 "${ROOT}/package/rc.engram-deployer" "${STAGE}/etc/rc.d/"

echo "==> Staging env sample..."
install -m 0644 "${ROOT}/package/engram-deployer.env.sample" \
  "${STAGE}/boot/config/plugins/engram-deployer/"

echo "==> Packing tarball..."
mkdir -p "$(dirname "$OUT")"
tar -czf "$OUT" -C "$STAGE" .

MD5="$(md5sum "$OUT" | awk '{print $1}')"
SIZE="$(stat -c %s "$OUT")"
echo ""
echo "==> Output: $OUT"
echo "    Size:   ${SIZE} bytes"
echo "    MD5:    ${MD5}"
echo ""
echo "Paste MD5 into engram-deployer.plg <ENTITY md5 ...> when cutting a release."
