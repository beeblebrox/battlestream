#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$SCRIPT_DIR/../streamdeck-plugin"
DIST="$PLUGIN_DIR/dist/com.battlestream.streamdeck.sdPlugin"

cd "$PLUGIN_DIR"
npm ci
npm run build

# Copy externalised native/CJS modules alongside the bundle
# (Rollup externalises them; Node.js resolves relative to bin/)
BIN_MODULES="$DIST/bin/node_modules"
mkdir -p "$BIN_MODULES"
cp -r node_modules/@napi-rs "$BIN_MODULES/"
cp -r node_modules/ws "$BIN_MODULES/"

echo "Plugin built at: $DIST"
