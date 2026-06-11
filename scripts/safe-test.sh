#!/usr/bin/env bash
# safe-test.sh — run go test in an isolated temp environment.
#
# Two layers of isolation:
#   1. BS_CONFIG_DIR / BS_DATA_DIR env vars point at a temp copy of the
#      config; internal/config honors BS_CONFIG_DIR via config.BaseDir().
#   2. HOME is overridden to the temp dir, so even code paths that derive
#      ~/.battlestream directly (e.g. via os.UserHomeDir) are isolated.
#
# The real ~/.battlestream/ (if it exists) is copied into the temp HOME so
# tests see a realistic config environment, and everything is cleaned up
# on exit. This prevents accidental reads or writes to the real config and
# database during development and testing.
#
# Usage:
#   scripts/safe-test.sh [go test flags and packages]
#
# Examples:
#   scripts/safe-test.sh ./...
#   scripts/safe-test.sh -run TestSomething ./internal/gamestate/
#   scripts/safe-test.sh -count=1 -v ./internal/store/

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPDIR_BASE="$(mktemp -d)"
CONFIG_DIR="$TMPDIR_BASE/.battlestream"
DATA_DIR="$TMPDIR_BASE/data"

cleanup() {
    rm -rf "$TMPDIR_BASE"
}
trap cleanup EXIT

mkdir -p "$CONFIG_DIR" "$DATA_DIR"

# Copy production config if it exists, so tests have a realistic config
# environment without risking writes to the real config dir. A missing
# ~/.battlestream is fine — tests run against an empty config dir.
PROD_CONFIG_DIR="${HOME}/.battlestream"
if [[ -d "$PROD_CONFIG_DIR" ]]; then
    cp -r "$PROD_CONFIG_DIR/." "$CONFIG_DIR/" 2>/dev/null || true
fi

# Pin the Go caches to their real locations BEFORE overriding HOME, so the
# build cache and module cache are not rebuilt/redownloaded under the temp
# HOME (and not deleted by cleanup).
export GOPATH="${GOPATH:-$(go env GOPATH)}"
export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"

# Layer 1: explicit overrides consumed by internal/config (config.BaseDir).
export BS_CONFIG_DIR="$CONFIG_DIR"
export BS_DATA_DIR="$DATA_DIR"

# Layer 2: belt-and-braces — any code that still derives ~/.battlestream
# from the home dir resolves into the temp dir instead.
export HOME="$TMPDIR_BASE"

cd "$REPO_ROOT"

# Default to all packages if none specified.
if [[ $# -eq 0 ]]; then
    set -- ./...
fi

echo "safe-test: isolated temp environment"
echo "  HOME=$HOME"
echo "  BS_CONFIG_DIR=$BS_CONFIG_DIR"
echo "  BS_DATA_DIR=$BS_DATA_DIR"
echo ""

go test -count=1 "$@"
