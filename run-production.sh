#!/bin/bash
#
# Production VPN Launcher
# =======================
# This script starts the VPN client with production settings:
# - Auto-reconnect is ALWAYS enabled (built into vpn-node)
# - Route all traffic through VPN (full VPN mode)
# - Proper logging and monitoring
#
# Auto-reconnect uses exponential backoff (1s to 30s).
# Reconnection count is tracked for uptime statistics.
#
# Usage:
#   ./run-production.sh                    # Interactive mode
#   ./run-production.sh --background       # Background daemon mode
#   ./run-production.sh --name my-client   # Custom client name
#

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VPN_SERVER="${VPN_SERVER:-95.217.238.72:443}"
VPN_NAME="${VPN_NAME:-$(hostname)}"
CONTROL_PORT="${CONTROL_PORT:-9001}"
UI_PORT="${UI_PORT:-8080}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_banner() {
    echo -e "${GREEN}"
    echo "╔═══════════════════════════════════════════════════╗"
    echo "║         VPN PRODUCTION LAUNCHER                    ║"
    echo "╠═══════════════════════════════════════════════════╣"
    echo "║  Server:        $VPN_SERVER"
    echo "║  Client Name:   $VPN_NAME"
    echo "║  Control Port:  $CONTROL_PORT"
    echo "║  UI Port:       $UI_PORT"
    echo "║  Auto-Reconnect: ALWAYS ON"
    echo "║  Route All:     ENABLED"
    echo "╚═══════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

check_binary() {
    if [ ! -f "$SCRIPT_DIR/bin/vpn-node" ]; then
        echo -e "${YELLOW}Binary not found. Building...${NC}"
        cd "$SCRIPT_DIR"
        make build
        # Sign on macOS
        if [ "$(uname)" = "Darwin" ]; then
            codesign --sign - --force --deep bin/vpn-node
            codesign --sign - --force --deep bin/vpn
        fi
    fi
}

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        echo -e "${RED}Error: This script must be run as root (sudo)${NC}"
        echo "VPN requires root privileges to create TUN device."
        echo ""
        echo "Usage: sudo ./run-production.sh"
        exit 1
    fi
}

parse_args() {
    BACKGROUND=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --background|-b)
                BACKGROUND=true
                shift
                ;;
            --name|-n)
                VPN_NAME="$2"
                shift 2
                ;;
            --server|-s)
                VPN_SERVER="$2"
                shift 2
                ;;
            --help|-h)
                echo "Usage: $0 [options]"
                echo ""
                echo "Options:"
                echo "  --background, -b    Run as background daemon"
                echo "  --name, -n NAME     Set client name (default: hostname)"
                echo "  --server, -s ADDR   Set server address (default: 95.217.238.72:443)"
                echo "  --help, -h          Show this help"
                echo ""
                echo "Environment variables:"
                echo "  VPN_SERVER          Server address (host:port)"
                echo "  VPN_NAME            Client name"
                echo "  CONTROL_PORT        Control socket port (default: 9001)"
                echo "  UI_PORT             Web UI port (default: 8080)"
                exit 0
                ;;
            *)
                echo -e "${RED}Unknown option: $1${NC}"
                exit 1
                ;;
        esac
    done
}

run_vpn() {
    print_banner

    echo -e "${GREEN}Starting VPN client in PRODUCTION mode...${NC}"
    echo ""
    echo "Production features (built-in):"
    echo "  - Auto-reconnect always enabled"
    echo "  - Exponential backoff (1s to 30s)"
    echo "  - Reconnection count tracked for statistics"
    echo ""

    if [ "$BACKGROUND" = true ]; then
        echo -e "${YELLOW}Running in background mode...${NC}"
        nohup "$SCRIPT_DIR/bin/vpn-node" \
            --connect "$VPN_SERVER" \
            --name "$VPN_NAME" \
            --listen-control "127.0.0.1:$CONTROL_PORT" \
            --listen-ui "localhost:$UI_PORT" \
            --route-all \
            > /var/log/vpn-node.log 2>&1 &

        echo "VPN started in background (PID: $!)"
        echo "Logs: /var/log/vpn-node.log"
        echo "UI: http://localhost:$UI_PORT"
        echo ""
        echo "To stop: sudo pkill -f vpn-node"
    else
        echo -e "${YELLOW}Running in foreground (Ctrl+C to stop)...${NC}"
        echo ""

        "$SCRIPT_DIR/bin/vpn-node" \
            --connect "$VPN_SERVER" \
            --name "$VPN_NAME" \
            --listen-control "127.0.0.1:$CONTROL_PORT" \
            --listen-ui "localhost:$UI_PORT" \
            --route-all
    fi
}

# Main
parse_args "$@"
check_root
check_binary
run_vpn
