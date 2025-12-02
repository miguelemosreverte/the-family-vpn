#!/bin/bash
# Core VPN service deployment script
# This is a COLD service - requires full restart

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Deploying Core VPN Service ==="
echo "Version: $(cat $SCRIPT_DIR/VERSION)"

# Build the binaries
echo "Building vpn-node..."
cd "$ROOT_DIR"
go build -o bin/vpn-node ./cmd/vpn-node

echo "Building vpn CLI..."
go build -o bin/vpn ./cmd/vpn

# If running on server, restart the service
if systemctl is-active --quiet vpn-node 2>/dev/null; then
    echo "Restarting vpn-node service..."
    sudo systemctl restart vpn-node
    echo "Service restarted"
else
    echo "vpn-node service not running (local development)"
fi

echo "=== Core VPN deployment complete ==="
