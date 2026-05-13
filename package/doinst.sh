#!/bin/sh
# doinst.sh — Slackware post-install hook, run by installpkg/upgradepkg after
# extraction with cwd=/. Runs as root. Idempotent so upgradepkg --install-new
# can re-run it safely.

set -e

PLUGIN_DIR=/boot/config/plugins/engram-deployer
CERT=$PLUGIN_DIR/cert.pem
KEY=$PLUGIN_DIR/key.pem

mkdir -p "$PLUGIN_DIR"

# Generate self-signed TLS cert on first install only. CI pins this cert's
# SHA-256 to detect mid-flight swaps.
if [ ! -f "$CERT" ] || [ ! -f "$KEY" ]; then
  echo "Generating self-signed TLS cert (10y, ed25519, CN=engram-deployer)..."
  openssl req -x509 -newkey ed25519 -nodes \
    -keyout "$KEY" -out "$CERT" \
    -days 3650 \
    -subj "/CN=engram-deployer" \
    -addext "subjectAltName=IP:10.0.20.214" \
    2>/dev/null
  chmod 0600 "$KEY"
  chmod 0644 "$CERT"
  echo "Cert SHA-256 (pin this in CI as DEPLOYER_CERT_PEM repo secret):"
  openssl x509 -in "$CERT" -noout -fingerprint -sha256
fi

ENV_FILE=$PLUGIN_DIR/engram-deployer.env
if [ ! -f "$ENV_FILE" ]; then
  echo "engram-deployer: edit ${PLUGIN_DIR}/engram-deployer.env.sample and"
  echo "                 rename to engram-deployer.env before starting the daemon."
else
  /etc/rc.d/rc.engram-deployer restart || true
fi
