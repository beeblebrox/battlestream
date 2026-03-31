#!/usr/bin/env bash
# backup-db.sh — copy the battlestream BadgerDB to a timestamped backup directory.
#
# Usage:
#   scripts/backup-db.sh [DB_PATH] [BACKUP_BASE]
#
# Defaults:
#   DB_PATH      ~/.battlestream/profiles/default/data
#   BACKUP_BASE  ~/.battlestream/backups
#
# Exits 0 on success. Idempotent: running twice with the same second-precision
# timestamp is safe (the second run skips if the destination already exists).

set -euo pipefail

DB_PATH="${1:-${HOME}/.battlestream/profiles/default/data}"
BACKUP_BASE="${2:-${HOME}/.battlestream/backups}"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DEST="${BACKUP_BASE}/${TIMESTAMP}"

if [[ ! -d "$DB_PATH" ]]; then
  echo "backup-db: DB path not found: $DB_PATH (nothing to back up)" >&2
  exit 0
fi

if [[ -d "$DEST" ]]; then
  echo "backup-db: destination already exists: $DEST (skipping)" >&2
  exit 0
fi

mkdir -p "$BACKUP_BASE"
cp -r "$DB_PATH" "$DEST"
echo "backup-db: backed up $DB_PATH -> $DEST"
