#!/usr/bin/env bash
# safe-test.sh — run go test in an isolated temp environment.
#
# Copies the battlestream config directory to a temp location, sets
# BS_CONFIG_DIR and BS_DATA_DIR env vars to that temp location, runs
# go test with those vars, and cleans up on exit.
#
# This prevents accidental reads or writes to the real ~/.battlestream/
# config and database during development and testing.
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
CONFIG_DIR="$TMPDIR_BASE/config"
DATA_DIR="$TMPDIR_BASE/data"

cleanup() {
    rm -rf "$TMPDIR_BASE"
}
trap cleanup EXIT

mkdir -p "$CONFIG_DIR" "$DATA_DIR"

# Copy production config if it exists, so tests have a realistic config
# environment without risking writes to the real config dir.
PROD_CONFIG_DIR="${HOME}/.battlestream"
if [[ -d "$PROD_CONFIG_DIR" ]]; then
    cp -r "$PROD_CONFIG_DIR/." "$CONFIG_DIR/" 2>/dev/null || true
fi

export BS_CONFIG_DIR="$CONFIG_DIR"
export BS_DATA_DIR="$DATA_DIR"

cd "$REPO_ROOT"

# Default to all packages if none specified.
if [[ $# -eq 0 ]]; then
    set -- ./...
fi

echo "safe-test: isolated temp environment"
echo "  BS_CONFIG_DIR=$BS_CONFIG_DIR"
echo "  BS_DATA_DIR=$BS_DATA_DIR"
echo ""

go test -count=1 "$@"
