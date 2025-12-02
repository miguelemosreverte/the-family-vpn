#!/bin/bash
# CLI service deployment script
# This is a HOT service - NO node restart required
# Only rebuilds the CLI binary, does not touch vpn-node

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Deploying CLI Service ==="
echo "Version: $(cat $SCRIPT_DIR/VERSION)"

# Find Go binary (same logic as deploy.go)
find_go() {
    for loc in \
        "$(which go 2>/dev/null)" \
        "/usr/local/go/bin/go" \
        "/usr/local/bin/go" \
        "/opt/homebrew/bin/go" \
        "/usr/bin/go" \
        "/root/go/bin/go"; do
        if [ -x "$loc" ]; then
            echo "$loc"
            return
        fi
    done
    echo "go"  # fallback to PATH
}

GO_BIN=$(find_go)
echo "Using Go: $GO_BIN"

# Build ONLY the CLI binary
echo "Building vpn CLI..."
cd "$ROOT_DIR"
$GO_BIN build -o bin/vpn ./cmd/vpn

# Sign on macOS
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Signing binary (macOS)..."
    codesign --sign - --force bin/vpn
fi

# NO RESTART - CLI updates don't need it
echo "=== CLI deployment complete (no restart needed) ==="
