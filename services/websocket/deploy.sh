#!/bin/bash
# WebSocket service deployment script
# This is a COLD service - requires restart (but separate from VPN tunnel)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Deploying WebSocket Service ==="
echo "Version: $(cat $SCRIPT_DIR/VERSION)"

# WebSocket is currently embedded in vpn-node
# In the future, this could be a separate service
echo "WebSocket service is currently part of vpn-node"
echo "Run services/core/deploy.sh to update"

echo "=== WebSocket deployment complete ==="
