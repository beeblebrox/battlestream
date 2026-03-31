#!/usr/bin/env bash
# hs-shop-shuffle.sh — Manipulate Hearthstone's "For You" shop page
# by clearing or editing the product tracking fields in options.txt.
#
# Usage:
#   hs-shop-shuffle.sh                  # interactive menu
#   hs-shop-shuffle.sh clear-seen       # clear "seen" list (cycle new products in)
#   hs-shop-shuffle.sh clear-all        # clear both seen + displayed lists
#   hs-shop-shuffle.sh backup           # just back up options.txt
#   hs-shop-shuffle.sh restore          # restore from latest backup
#   hs-shop-shuffle.sh list             # show current product IDs
#
# IMPORTANT: Hearthstone must be CLOSED before running this.
# The game overwrites options.txt on exit.

set -euo pipefail

# Default path — override with HS_OPTIONS_PATH env var
OPTIONS_FILE="${HS_OPTIONS_PATH:-/chungus/battlenet/drive_c/users/steamuser/AppData/Local/Blizzard/Hearthstone/options.txt}"
BACKUP_DIR="${OPTIONS_FILE%/*}/backups"

die() { echo "ERROR: $*" >&2; exit 1; }

check_file() {
    [[ -f "$OPTIONS_FILE" ]] || die "options.txt not found at: $OPTIONS_FILE"
}

check_hs_not_running() {
    if pgrep -f "Hearthstone.exe" >/dev/null 2>&1; then
        die "Hearthstone appears to be running. Close it first — it overwrites options.txt on exit."
    fi
}

backup() {
    mkdir -p "$BACKUP_DIR"
    local ts
    ts=$(date +%Y%m%d_%H%M%S)
    local dest="$BACKUP_DIR/options_${ts}.txt"
    cp "$OPTIONS_FILE" "$dest"
    echo "Backed up to: $dest"
}

restore() {
    local latest
    latest=$(ls -t "$BACKUP_DIR"/options_*.txt 2>/dev/null | head -1)
    [[ -n "$latest" ]] || die "No backups found in $BACKUP_DIR"
    cp "$latest" "$OPTIONS_FILE"
    echo "Restored from: $latest"
}

count_ids() {
    local field="$1"
    local line
    line=$(grep "^${field}=" "$OPTIONS_FILE" 2>/dev/null || true)
    if [[ -z "$line" || "$line" == "${field}=" ]]; then
        echo 0
    else
        local val="${line#*=}"
        # Strip version prefix (e.g. "2203|...")
        val="${val#*|}"
        echo "$val" | tr ':' '\n' | grep -c '_' 2>/dev/null || echo 0
    fi
}

list_ids() {
    echo "=== latestDisplayedShopProductList ==="
    local displayed
    displayed=$(grep "^latestDisplayedShopProductList=" "$OPTIONS_FILE" 2>/dev/null || true)
    if [[ -n "$displayed" ]]; then
        local val="${displayed#*=}"
        local version="${val%%|*}"
        local ids="${val#*|}"
        local count
        count=$(echo "$ids" | tr ':' '\n' | grep -c '_' 2>/dev/null || echo 0)
        echo "  Version: $version"
        echo "  Products: $count"
        echo "  First 10 IDs:"
        echo "$ids" | tr ':' '\n' | head -10 | sed 's/^/    /'
    else
        echo "  (not set)"
    fi

    echo ""
    echo "=== latestSeenShopProductList ==="
    local seen
    seen=$(grep "^latestSeenShopProductList=" "$OPTIONS_FILE" 2>/dev/null || true)
    if [[ -n "$seen" ]]; then
        local val="${seen#*=}"
        local version="${val%%|*}"
        local ids="${val#*|}"
        local count
        count=$(echo "$ids" | tr ':' '\n' | grep -c '[0-9]' 2>/dev/null || echo 0)
        echo "  Version: $version"
        echo "  Products: $count"
        echo "  First 10 IDs:"
        echo "$ids" | tr ':' '\n' | head -10 | sed 's/^/    /'
    else
        echo "  (not set)"
    fi
}

clear_seen() {
    check_hs_not_running
    backup
    if grep -q "^latestSeenShopProductList=" "$OPTIONS_FILE"; then
        sed -i 's/^latestSeenShopProductList=.*/latestSeenShopProductList=/' "$OPTIONS_FILE"
        echo "Cleared latestSeenShopProductList"
    else
        echo "latestSeenShopProductList not found in options.txt"
    fi
    echo "Next time you open the shop, products should appear as 'new' and rotate differently."
}

clear_displayed() {
    check_hs_not_running
    backup
    if grep -q "^latestDisplayedShopProductList=" "$OPTIONS_FILE"; then
        sed -i 's/^latestDisplayedShopProductList=.*/latestDisplayedShopProductList=/' "$OPTIONS_FILE"
        echo "Cleared latestDisplayedShopProductList"
    else
        echo "latestDisplayedShopProductList not found in options.txt"
    fi
}

clear_all() {
    check_hs_not_running
    backup
    clear_seen
    clear_displayed
    echo ""
    echo "Both lists cleared. The shop should show a fresh rotation on next launch."
}

interactive() {
    check_file
    echo "Hearthstone Shop Shuffler"
    echo "========================="
    echo "Options file: $OPTIONS_FILE"
    echo ""

    local d_count s_count
    d_count=$(count_ids "latestDisplayedShopProductList")
    s_count=$(count_ids "latestSeenShopProductList")
    echo "Currently tracking: $d_count displayed, $s_count seen products"
    echo ""
    echo "1) Clear 'seen' list (cycle new products into For You page)"
    echo "2) Clear both lists (full reset)"
    echo "3) Show current product IDs"
    echo "4) Backup options.txt"
    echo "5) Restore from backup"
    echo "6) Quit"
    echo ""
    read -rp "Choice [1-6]: " choice

    case "$choice" in
        1) clear_seen ;;
        2) clear_all ;;
        3) list_ids ;;
        4) backup ;;
        5) restore ;;
        6) exit 0 ;;
        *) die "Invalid choice" ;;
    esac
}

# Main
check_file

case "${1:-}" in
    clear-seen)    clear_seen ;;
    clear-all)     clear_all ;;
    backup)        backup ;;
    restore)       restore ;;
    list)          list_ids ;;
    "")            interactive ;;
    *)             die "Unknown command: $1. Use: clear-seen, clear-all, backup, restore, list" ;;
esac
