#!/bin/bash
# Sign a Windows PE binary with osslsigncode.
# Called by GoReleaser post-build hook.
set -euo pipefail

FILE="$1"
OS="$2"

if [ "$OS" != "windows" ]; then
  exit 0
fi

if [ -z "${WIN_CERT_PFX:-}" ]; then
  echo "WIN_CERT_PFX not set, skipping Windows signing"
  exit 0
fi

# Decode cert if base64-encoded
CERT_FILE=$(mktemp)
echo "$WIN_CERT_PFX" | base64 -d > "$CERT_FILE"
trap "rm -f $CERT_FILE" EXIT

osslsigncode sign \
  -pkcs12 "$CERT_FILE" \
  -pass "$WIN_CERT_PASSWORD" \
  -h sha256 \
  -ts http://timestamp.digicert.com \
  -in "$FILE" \
  -out "${FILE}.signed"

mv "${FILE}.signed" "$FILE"
