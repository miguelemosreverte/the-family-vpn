#!/bin/bash
#
# VPN Update Script
# This script runs every 5 minutes to pull updates and restart the VPN if needed.
#

INSTALL_DIR="$HOME/the-family-vpn"
LOG_FILE="/var/log/vpn-update.log"
LOCK_FILE="/tmp/vpn-update.lock"

# Ensure only one instance runs (use mkdir for portability - flock not on all systems)
if ! mkdir "$LOCK_FILE" 2>/dev/null; then
    # Check if lock is stale (older than 5 minutes)
    if [[ -d "$LOCK_FILE" ]]; then
        find "$LOCK_FILE" -mmin +5 -exec rm -rf {} \; 2>/dev/null
        mkdir "$LOCK_FILE" 2>/dev/null || exit 0
    else
        exit 0
    fi
fi
trap "rm -rf $LOCK_FILE" EXIT

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | sudo tee -a "$LOG_FILE" > /dev/null
}

cd "$INSTALL_DIR" || exit 1

# Fetch updates
git fetch origin 2>/dev/null

# Check if there are updates
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)

if [[ "$LOCAL" != "$REMOTE" ]]; then
    log "Updates available, pulling..."

    # Pull updates
    git reset --hard origin/main

    # Ensure Go is in PATH
    export PATH=$PATH:/usr/local/go/bin

    # Find Go binary
    GO_CMD="go"
    if ! command -v go &> /dev/null; then
        GO_CMD="/usr/local/go/bin/go"
    fi

    # Rebuild binaries
    log "Rebuilding binaries..."
    $GO_CMD build -o bin/vpn-node ./cmd/vpn-node 2>&1 | sudo tee -a "$LOG_FILE"
    $GO_CMD build -o bin/vpn ./cmd/vpn 2>&1 | sudo tee -a "$LOG_FILE"

    # Sign on macOS (CRITICAL for TUN device access)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        log "Signing binaries for macOS..."
        codesign --sign - --force --deep bin/vpn-node 2>&1 | sudo tee -a "$LOG_FILE"
        codesign --sign - --force --deep bin/vpn 2>&1 | sudo tee -a "$LOG_FILE"
    fi

    # Kill ALL existing vpn-node processes before restart
    log "Stopping all VPN processes..."
    sudo pkill -9 -f "vpn-node" 2>/dev/null || true
    sleep 2

    # Restart VPN service
    log "Starting VPN service..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null || true
        sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-ui.plist 2>/dev/null || true
        sleep 1
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-ui.plist 2>/dev/null || true
    else
        sudo systemctl restart vpn-node
        sudo systemctl restart vpn-ui 2>/dev/null || true
    fi

    log "Update complete: $(git log -1 --oneline)"
else
    log "No updates available"
fi

# Ensure VPN is running (even if no updates)
if ! pgrep -f "vpn-node" > /dev/null; then
    log "VPN not running, starting..."

    # Make sure binaries are signed on macOS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        codesign --sign - --force --deep bin/vpn-node 2>/dev/null || true
        codesign --sign - --force --deep bin/vpn 2>/dev/null || true
    fi

    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null || true
    else
        sudo systemctl start vpn-node
    fi
fi

log "Check complete"
