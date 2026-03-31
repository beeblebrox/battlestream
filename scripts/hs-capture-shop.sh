#!/usr/bin/env bash
# hs-capture-shop.sh — Capture Hearthstone network traffic to investigate
# the shop product catalog served by Blizzard's servers.
#
# Usage:
#   sudo ./hs-capture-shop.sh start          # start capture, then launch HS
#   sudo ./hs-capture-shop.sh stop           # stop capture
#   ./hs-capture-shop.sh analyze [file.pcap] # analyze a capture (no sudo needed)
#
# The capture targets Blizzard/Activision IP ranges and common service ports.
# Product catalog data is likely served over HTTPS (encrypted), but we can
# still identify endpoints, timing, and payload sizes to understand the flow.

set -euo pipefail

CAPTURE_DIR="/tmp/hs-captures"
PIDFILE="$CAPTURE_DIR/tcpdump.pid"

# Blizzard IP ranges — comprehensive list from first capture's DNS results
# plus known Blizzard ASN ranges
BLIZZARD_NETS=(
    "137.221.0.0/16"      # Blizzard owned (us.version.battle.net, us.actual.battle.net)
    "24.105.0.0/18"       # Blizzard owned
    "37.244.28.0/24"      # Blizzard EU
    "37.244.30.0/23"      # Blizzard EU
    "185.60.112.0/22"     # Blizzard
    "195.12.234.0/23"     # Blizzard
    "199.108.0.0/18"      # Blizzard
    "5.42.168.0/21"       # Blizzard
    "34.16.0.0/14"        # GCP (us.actual.battle.net -> 34.16.239.68)
    "34.120.0.0/14"       # GCP (us.actual.battle.net -> 34.125.251.198)
    "75.2.95.0/24"        # AWS GA (api.blizzard.com)
    "99.83.136.0/24"      # AWS GA (api.blizzard.com)
    "54.201.115.0/24"     # AWS (oauth.battle.net)
    "35.167.3.0/24"       # AWS (oauth.battle.net)
    "50.112.47.0/24"      # AWS (oauth.battle.net)
    "151.101.0.0/16"      # Fastly CDN (cdn.blz-contentstack.com)
    "23.215.55.0/24"      # Akamai (blz-contentstack-images.akamaized.net)
)

die() { echo "ERROR: $*" >&2; exit 1; }

start_capture() {
    [[ $EUID -eq 0 ]] || die "Must run with sudo for packet capture"

    mkdir -p "$CAPTURE_DIR"
    local ts
    ts=$(date +%Y%m%d_%H%M%S)
    local pcap="$CAPTURE_DIR/hs_shop_${ts}.pcap"

    # Build BPF filter for Blizzard networks
    local filter=""
    for net in "${BLIZZARD_NETS[@]}"; do
        if [[ -n "$filter" ]]; then
            filter="$filter or "
        fi
        filter="${filter}net $net"
    done

    # Also capture DNS lookups for Blizzard domains
    filter="($filter) or (port 53)"

    echo "Starting capture -> $pcap"
    echo "Filter: $filter"
    echo ""
    echo "Now launch Hearthstone, open the shop, browse the 'For You' page,"
    echo "then come back and run: sudo $0 stop"
    echo ""

    # Find the main network interface
    local iface
    iface=$(ip route show default | awk '/default/ {print $5}' | head -1)
    [[ -n "$iface" ]] || die "Could not detect default network interface"
    echo "Capturing on interface: $iface"

    tcpdump -i "$iface" -w "$pcap" -s 0 "$filter" &
    echo $! > "$PIDFILE"
    echo "tcpdump PID: $(cat "$PIDFILE")"
    echo "Capture file: $pcap"
}

stop_capture() {
    [[ $EUID -eq 0 ]] || die "Must run with sudo"

    if [[ -f "$PIDFILE" ]]; then
        local pid
        pid=$(cat "$PIDFILE")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
            wait "$pid" 2>/dev/null || true
            echo "Stopped tcpdump (PID $pid)"
        else
            echo "tcpdump process $pid already stopped"
        fi
        rm -f "$PIDFILE"
    else
        echo "No PID file found, trying to kill any tcpdump..."
        pkill tcpdump 2>/dev/null && echo "Killed tcpdump" || echo "No tcpdump running"
    fi

    local latest
    latest=$(ls -t "$CAPTURE_DIR"/hs_shop_*.pcap 2>/dev/null | head -1)
    if [[ -n "$latest" ]]; then
        local size
        size=$(stat -c%s "$latest" 2>/dev/null || stat -f%z "$latest" 2>/dev/null)
        echo ""
        echo "Capture file: $latest"
        echo "Size: $((size / 1024)) KB"
        echo ""
        echo "To analyze: $0 analyze $latest"
    fi
}

analyze_capture() {
    local pcap="${1:-}"
    if [[ -z "$pcap" ]]; then
        pcap=$(ls -t "$CAPTURE_DIR"/hs_shop_*.pcap 2>/dev/null | head -1)
        [[ -n "$pcap" ]] || die "No capture file found. Specify path or run a capture first."
    fi
    [[ -f "$pcap" ]] || die "File not found: $pcap"

    echo "=== Capture Summary ==="
    echo "File: $pcap"
    echo ""

    echo "--- DNS queries (Blizzard domains) ---"
    tcpdump -r "$pcap" -nn port 53 2>/dev/null | grep -iE 'blizzard|battle\.net|blz-contentstack|bnet' | head -30 || echo "(none found)"
    echo ""

    echo "--- Unique destination IPs + ports ---"
    tcpdump -r "$pcap" -nn 'not port 53' 2>/dev/null | \
        grep -oP '> \K[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | \
        sort -u | head -30 || echo "(none)"
    echo ""

    echo "--- TLS SNI (Server Name Indication) hostnames ---"
    # Extract SNI from TLS Client Hello packets
    tcpdump -r "$pcap" -nn -A 2>/dev/null | \
        grep -aoP '[\w.-]+\.blizzard\.com|[\w.-]+\.battle\.net|[\w.-]+\.blz-contentstack\.com' | \
        sort -u || echo "(none found — traffic may be pinned or use IP-only)"
    echo ""

    echo "--- Largest responses (possible product catalog) ---"
    tcpdump -r "$pcap" -nn 'not port 53' 2>/dev/null | \
        awk '{print $NF, $0}' | sort -rn | head -10 || echo "(none)"
    echo ""

    echo "--- Connection timeline (first packet per flow) ---"
    tcpdump -r "$pcap" -nn 'tcp[tcpflags] & tcp-syn != 0 and not port 53' 2>/dev/null | head -20 || echo "(none)"
    echo ""

    echo "To dig deeper, open in Wireshark:"
    echo "  wireshark $pcap"
}

case "${1:-}" in
    start)   start_capture ;;
    stop)    stop_capture ;;
    analyze) analyze_capture "${2:-}" ;;
    *)
        echo "Usage: $0 {start|stop|analyze [file.pcap]}"
        echo ""
        echo "Workflow:"
        echo "  1. Close Hearthstone"
        echo "  2. sudo $0 start"
        echo "  3. Launch Hearthstone, open shop, browse For You page"
        echo "  4. sudo $0 stop"
        echo "  5. $0 analyze"
        ;;
esac
