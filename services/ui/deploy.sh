#!/bin/bash
# UI service deployment script
# This is a HOT service - NO node restart required
# The UI is served by the CLI (vpn ui command), so only CLI rebuild needed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Deploying UI Service ==="
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

# Build ONLY the CLI binary (which contains the UI)
echo "Building vpn CLI (includes UI)..."
cd "$ROOT_DIR"
$GO_BIN build -o bin/vpn ./cmd/vpn

# Sign on macOS
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Signing binary (macOS)..."
    codesign --sign - --force bin/vpn
fi

# NO RESTART - UI updates don't need it
# If UI is currently running, user can restart it manually
echo "=== UI deployment complete (no restart needed) ==="
echo "Note: If 'vpn ui' is running, restart it to see changes"
