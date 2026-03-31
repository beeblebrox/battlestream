#!/usr/bin/env bash
# progress-report.sh — Generate a new BattleStream progress report skeleton
#
# Usage:
#   scripts/progress-report.sh <slug> [--title "Title text"] [--tags feat,ui,api]
#   scripts/progress-report.sh my-feature
#   scripts/progress-report.sh fix-placement-chart --title "Fix placement chart axis labels" --tags fix,ui
#
# The slug becomes part of the filename: YYYY-MM-DD-HHMMSS-<slug>.html
# A screenshots/ subdirectory is created alongside the report for images.
# The master .progress/index.html is updated with the new report entry.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROGRESS_DIR="$REPO_ROOT/.progress"
TEMPLATE="$PROGRESS_DIR/_template.html"
INDEX="$PROGRESS_DIR/index.html"

# ── Argument parsing ─────────────────────────────────────────────────────────
SLUG=""
TITLE=""
TAGS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --title)
      TITLE="$2"
      shift 2
      ;;
    --tags)
      TAGS="$2"
      shift 2
      ;;
    --help|-h)
      sed -n '2,12p' "$0" | sed 's/^# //'
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
    *)
      if [[ -z "$SLUG" ]]; then
        SLUG="$1"
      else
        echo "Unexpected argument: $1" >&2
        exit 1
      fi
      shift
      ;;
  esac
done

if [[ -z "$SLUG" ]]; then
  echo "Usage: $0 <slug> [--title \"Title\"] [--tags feat,ui,api]" >&2
  echo "  Example: $0 fix-placement-chart --title \"Fix placement chart\" --tags fix,ui" >&2
  exit 1
fi

# Sanitize slug: lowercase, alphanumeric + dashes only
SLUG="$(echo "$SLUG" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//')"

if [[ -z "$SLUG" ]]; then
  echo "Slug '$1' produced an empty sanitized name. Use alphanumeric characters and dashes." >&2
  exit 1
fi

# ── Timestamp ────────────────────────────────────────────────────────────────
TIMESTAMP="$(date '+%Y-%m-%d-%H%M%S')"
ISO_DATE="$(date '+%Y-%m-%dT%H:%M:%S')"
DISPLAY_DATE="$(date '+%Y-%m-%d %H:%M:%S')"

REPORT_FILENAME="${TIMESTAMP}-${SLUG}.html"
REPORT_PATH="$PROGRESS_DIR/$REPORT_FILENAME"
SCREENSHOTS_DIR="$PROGRESS_DIR/screenshots/${TIMESTAMP}-${SLUG}"

# ── Derived title ─────────────────────────────────────────────────────────────
if [[ -z "$TITLE" ]]; then
  # Convert slug to title case
  TITLE="$(echo "$SLUG" | sed 's/-/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2); print}')"
fi

# ── Check prerequisites ──────────────────────────────────────────────────────
if [[ ! -f "$TEMPLATE" ]]; then
  echo "Template not found: $TEMPLATE" >&2
  echo "Run this script from the repo root or ensure .progress/_template.html exists." >&2
  exit 1
fi

if [[ ! -f "$INDEX" ]]; then
  echo "Index not found: $INDEX" >&2
  exit 1
fi

# ── Create report from template ───────────────────────────────────────────────
echo "Creating report: $REPORT_FILENAME"
sed \
  -e "s|REPORT_TITLE|$TITLE|g" \
  -e "s|REPORT_DATE|$DISPLAY_DATE|g" \
  -e "s|REPORT_SUMMARY|<!-- Add a one-sentence summary here -->|g" \
  "$TEMPLATE" > "$REPORT_PATH"

# ── Create screenshots directory ──────────────────────────────────────────────
mkdir -p "$SCREENSHOTS_DIR"
# Create a .gitkeep so screenshots dir is preserved if empty
touch "$SCREENSHOTS_DIR/.gitkeep"
echo "Screenshots dir: $SCREENSHOTS_DIR"

# ── Inject report into index.html ────────────────────────────────────────────
# Build the new REPORTS entry as a JS object
TAGS_JSON="[]"
if [[ -n "$TAGS" ]]; then
  # Convert comma-separated tags to JSON array: "feat,ui" -> ["feat","ui"]
  TAGS_JSON="[$(echo "$TAGS" | sed 's/,/","/g' | sed 's/^/"/' | sed 's/$/"/' )]"
fi

NEW_ENTRY="      { file: \"$REPORT_FILENAME\", date: \"$ISO_DATE\", title: $(printf '%s' "$TITLE" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'), summary: \"\", tags: $TAGS_JSON, screenshots: 0, diffs: 0 },"

# Insert the new entry just after the REPORTS_PLACEHOLDER comment
python3 - "$INDEX" "$NEW_ENTRY" <<'PYEOF'
import sys, re

index_path = sys.argv[1]
new_entry  = sys.argv[2]

with open(index_path, 'r') as f:
    content = f.read()

placeholder = '      // REPORTS_PLACEHOLDER — do not remove this comment'
if placeholder not in content:
    print(f"WARNING: placeholder not found in {index_path}; entry not injected.", file=sys.stderr)
    sys.exit(0)

replacement = placeholder + '\n' + new_entry
content = content.replace(placeholder, replacement, 1)

with open(index_path, 'w') as f:
    f.write(content)

print("Index updated.")
PYEOF

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "Done!"
echo "  Report:      .progress/$REPORT_FILENAME"
echo "  Screenshots: .progress/screenshots/${TIMESTAMP}-${SLUG}/"
echo "  Index:       .progress/index.html"
echo ""
echo "Next steps:"
echo "  1. Edit the report:     \$EDITOR .progress/$REPORT_FILENAME"
echo "  2. Add screenshots to:  .progress/screenshots/${TIMESTAMP}-${SLUG}/"
echo "  3. Update the summary field in .progress/index.html for this entry."
echo ""
echo "To view: open .progress/index.html in a browser."
