#!/bin/bash
# setup-client.sh - Set up VPN client on a new machine
#
# Usage:
#   ./scripts/setup-client.sh                    # Interactive setup
#   ./scripts/setup-client.sh --name mac-mini    # Named setup
#   ./scripts/setup-client.sh --remote user@host # Remote setup via SSH

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SERVER_HOST="95.217.238.72"
SERVER_PORT="443"
REPO_URL="https://github.com/miguelemosreverte/the-family-vpn.git"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${GREEN}[SETUP]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }

# Parse arguments
NODE_NAME=""
REMOTE_HOST=""
VPN_ADDRESS=""
AUTO_START=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --name|-n)
            NODE_NAME="$2"
            shift 2
            ;;
        --remote|-r)
            REMOTE_HOST="$2"
            shift 2
            ;;
        --vpn-addr|-a)
            VPN_ADDRESS="$2"
            shift 2
            ;;
        --auto-start)
            AUTO_START=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--name NAME] [--remote user@host] [--vpn-addr IP] [--auto-start]"
            exit 1
            ;;
    esac
done

setup_local() {
    log "Setting up VPN client locally..."

    # Check for Go
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Install from https://golang.org/dl/"
    fi

    # Default name to hostname
    if [ -z "$NODE_NAME" ]; then
        NODE_NAME=$(hostname | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g')
    fi

    cd "$PROJECT_DIR"

    # Build binaries
    log "Building binaries..."
    go build -o bin/vpn-node ./cmd/vpn-node
    go build -o bin/vpn ./cmd/vpn

    # Create data directory
    mkdir -p ~/.vpn-node

    # Create systemd service (Linux) or launchd plist (macOS)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        create_launchd_service
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        create_systemd_service
    fi

    log "Setup complete!"
    echo ""
    info "To start the VPN client manually:"
    echo "  sudo $PROJECT_DIR/bin/vpn-node --connect $SERVER_HOST:$SERVER_PORT --name $NODE_NAME"
    echo ""
    info "To route all traffic through VPN:"
    echo "  sudo $PROJECT_DIR/bin/vpn-node --connect $SERVER_HOST:$SERVER_PORT --name $NODE_NAME --route-all"
    echo ""
    info "To start the dashboard:"
    echo "  $PROJECT_DIR/bin/vpn ui"
}

create_launchd_service() {
    local PLIST_PATH="$HOME/Library/LaunchAgents/com.family-vpn.node.plist"
    local VPN_NODE_PATH="$PROJECT_DIR/bin/vpn-node"

    log "Creating launchd service for macOS..."

    mkdir -p "$(dirname "$PLIST_PATH")"

    cat > "$PLIST_PATH" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family-vpn.node</string>
    <key>ProgramArguments</key>
    <array>
        <string>$VPN_NODE_PATH</string>
        <string>--connect</string>
        <string>$SERVER_HOST:$SERVER_PORT</string>
        <string>--name</string>
        <string>$NODE_NAME</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>/tmp/vpn-node.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/vpn-node.log</string>
</dict>
</plist>
PLIST

    info "Launchd service created at: $PLIST_PATH"
    info "To start: launchctl load $PLIST_PATH"
    info "To stop: launchctl unload $PLIST_PATH"
    info "Note: VPN requires root, so manual start with sudo is recommended"
}

create_systemd_service() {
    local SERVICE_PATH="/etc/systemd/system/vpn-node.service"
    local VPN_NODE_PATH="$PROJECT_DIR/bin/vpn-node"

    log "Creating systemd service for Linux..."

    sudo tee "$SERVICE_PATH" > /dev/null << SERVICE
[Unit]
Description=Family VPN Node
After=network.target

[Service]
Type=simple
ExecStart=$VPN_NODE_PATH --connect $SERVER_HOST:$SERVER_PORT --name $NODE_NAME
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICE

    sudo systemctl daemon-reload

    info "Systemd service created"
    info "To enable at boot: sudo systemctl enable vpn-node"
    info "To start: sudo systemctl start vpn-node"
    info "To check status: sudo systemctl status vpn-node"
}

setup_remote() {
    log "Setting up VPN client on remote host: $REMOTE_HOST"

    # Default name from remote hostname
    if [ -z "$NODE_NAME" ]; then
        NODE_NAME=$(ssh "$REMOTE_HOST" 'hostname' | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g')
    fi

    # Check if Go is installed on remote
    if ! ssh "$REMOTE_HOST" 'command -v go' &> /dev/null; then
        warn "Go not installed on remote. Installing..."
        ssh "$REMOTE_HOST" 'bash -s' << 'INSTALL_GO'
            # Try brew first (macOS)
            if command -v brew &> /dev/null; then
                brew install go
            # Then apt (Debian/Ubuntu)
            elif command -v apt-get &> /dev/null; then
                sudo apt-get update && sudo apt-get install -y golang
            else
                echo "Please install Go manually: https://golang.org/dl/"
                exit 1
            fi
INSTALL_GO
    fi

    # Clone or update repo on remote
    log "Setting up repository on remote..."
    ssh "$REMOTE_HOST" "bash -s" << REMOTE_SETUP
        set -e
        if [ -d ~/the-family-vpn ]; then
            cd ~/the-family-vpn && git pull origin main
        else
            git clone $REPO_URL ~/the-family-vpn
        fi
        cd ~/the-family-vpn
        go build -o bin/vpn-node ./cmd/vpn-node
        go build -o bin/vpn ./cmd/vpn
        mkdir -p ~/.vpn-node
REMOTE_SETUP

    log "Remote setup complete!"
    echo ""
    info "To start VPN on remote:"
    echo "  ssh $REMOTE_HOST 'sudo ~/the-family-vpn/bin/vpn-node --connect $SERVER_HOST:$SERVER_PORT --name $NODE_NAME'"
    echo ""
    info "To start with route-all:"
    echo "  ssh $REMOTE_HOST 'sudo ~/the-family-vpn/bin/vpn-node --connect $SERVER_HOST:$SERVER_PORT --name $NODE_NAME --route-all'"
}

# Main
if [ -n "$REMOTE_HOST" ]; then
    setup_remote
else
    setup_local
fi
