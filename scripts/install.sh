#!/bin/bash
#
# VPN Client Installation Script
# Run this script after cloning the repository to set up and start the VPN client.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/miguelemosreverte/the-family-vpn/main/scripts/install.sh | bash
#   OR
#   git clone https://github.com/miguelemosreverte/the-family-vpn.git && cd the-family-vpn && ./scripts/install.sh
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
VPN_SERVER="95.217.238.72:443"
REPO_URL="https://github.com/miguelemosreverte/the-family-vpn.git"
INSTALL_DIR="$HOME/the-family-vpn"

print_header() {
    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_step() {
    echo -e "${GREEN}▶${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✖${NC} $1"
}

print_success() {
    echo -e "${GREEN}✔${NC} $1"
}

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        ARCH=$(uname -m)
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        OS="linux"
        ARCH=$(uname -m)
    else
        print_error "Unsupported operating system: $OSTYPE"
        exit 1
    fi
    print_step "Detected: $OS ($ARCH)"
}

# Check if running as root (we'll need sudo for TUN device)
check_sudo() {
    if ! sudo -n true 2>/dev/null; then
        print_warning "This script requires sudo access to create the VPN tunnel."
        print_warning "You will be prompted for your password."
        echo ""
    fi
}

# Install Go if not present
install_go() {
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | awk '{print $3}')
        print_success "Go is already installed: $GO_VERSION"
        return
    fi

    print_step "Installing Go..."

    if [[ "$OS" == "macos" ]]; then
        if [[ "$ARCH" == "arm64" ]]; then
            GO_PKG="go1.22.0.darwin-arm64.pkg"
        else
            GO_PKG="go1.22.0.darwin-amd64.pkg"
        fi

        curl -L -o /tmp/go.pkg "https://go.dev/dl/$GO_PKG"
        sudo installer -pkg /tmp/go.pkg -target /
        rm /tmp/go.pkg

        # Add to PATH
        export PATH=$PATH:/usr/local/go/bin

    elif [[ "$OS" == "linux" ]]; then
        if [[ "$ARCH" == "aarch64" ]]; then
            GO_TAR="go1.22.0.linux-arm64.tar.gz"
        else
            GO_TAR="go1.22.0.linux-amd64.tar.gz"
        fi

        curl -L -o /tmp/go.tar.gz "https://go.dev/dl/$GO_TAR"
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz

        # Add to PATH
        export PATH=$PATH:/usr/local/go/bin

        # Add to profile if not already there
        if ! grep -q '/usr/local/go/bin' ~/.profile 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
        fi
    fi

    print_success "Go installed successfully"
}

# Clone or update repository
setup_repository() {
    if [[ -d "$INSTALL_DIR" ]]; then
        print_step "Updating existing repository..."
        cd "$INSTALL_DIR"
        git pull origin main
    else
        print_step "Cloning repository..."
        git clone "$REPO_URL" "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi
    print_success "Repository ready at $INSTALL_DIR"
}

# Build binaries
build_binaries() {
    print_step "Building VPN binaries..."
    cd "$INSTALL_DIR"

    # Ensure Go is in PATH
    export PATH=$PATH:/usr/local/go/bin

    mkdir -p bin
    go build -o bin/vpn-node ./cmd/vpn-node
    go build -o bin/vpn ./cmd/vpn

    # Sign binaries on macOS (required for TUN device access)
    if [[ "$OS" == "macos" ]]; then
        print_step "Signing binaries for macOS..."
        codesign --sign - --force --deep bin/vpn-node
        codesign --sign - --force --deep bin/vpn
    fi

    print_success "Binaries built successfully"
}

# Get node name (hostname without special chars)
get_node_name() {
    NODE_NAME=$(hostname | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/--*/-/g' | sed 's/^-//' | sed 's/-$//')
    if [[ -z "$NODE_NAME" ]]; then
        NODE_NAME="vpn-client"
    fi
    echo "$NODE_NAME"
}

# Install launchd service (macOS)
install_macos_service() {
    print_step "Installing launchd service for auto-start..."

    NODE_NAME=$(get_node_name)
    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-node.plist"

    # Create plist
    sudo tee "$PLIST_PATH" > /dev/null << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn-node</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/bin/vpn-node</string>
        <string>--connect</string>
        <string>$VPN_SERVER</string>
        <string>--name</string>
        <string>$NODE_NAME</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>NetworkState</key>
        <true/>
    </dict>
    <key>StandardOutPath</key>
    <string>/var/log/vpn-node.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/vpn-node.log</string>
    <key>WorkingDirectory</key>
    <string>$INSTALL_DIR</string>
</dict>
</plist>
EOF

    sudo chown root:wheel "$PLIST_PATH"
    sudo chmod 644 "$PLIST_PATH"

    print_success "launchd service installed"
}

# Install systemd service (Linux)
install_linux_service() {
    print_step "Installing systemd service for auto-start..."

    NODE_NAME=$(get_node_name)
    SERVICE_PATH="/etc/systemd/system/vpn-node.service"

    sudo tee "$SERVICE_PATH" > /dev/null << EOF
[Unit]
Description=Family VPN Node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/vpn-node --connect $VPN_SERVER --name $NODE_NAME
Restart=always
RestartSec=10
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-node

    print_success "systemd service installed"
}

# Start the VPN service
start_vpn() {
    print_step "Starting VPN client..."

    # Stop any existing VPN process
    sudo pkill -f "vpn-node" 2>/dev/null || true
    sleep 1

    if [[ "$OS" == "macos" ]]; then
        # Load and start launchd service
        sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null || true
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist
    else
        # Start systemd service
        sudo systemctl start vpn-node
    fi

    # Wait for connection
    sleep 3
}

# Verify VPN is working
verify_connection() {
    print_step "Verifying VPN connection..."

    cd "$INSTALL_DIR"

    # Check if process is running
    if pgrep -f "vpn-node" > /dev/null; then
        print_success "VPN process is running"
    else
        print_error "VPN process is not running!"
        echo ""
        echo "Check logs with: sudo cat /var/log/vpn-node.log"
        return 1
    fi

    # Try to get status
    sleep 2
    if ./bin/vpn status 2>/dev/null | grep -q "Connected"; then
        print_success "VPN is connected!"
        echo ""
        ./bin/vpn status
    else
        print_warning "VPN may still be connecting..."
        echo ""
        echo "Check status with: $INSTALL_DIR/bin/vpn status"
    fi
}

# Show final instructions
show_instructions() {
    NODE_NAME=$(get_node_name)

    print_header "Installation Complete!"

    echo -e "${GREEN}Your VPN client is now installed and running.${NC}"
    echo ""
    echo "Node name: $NODE_NAME"
    echo "Server: $VPN_SERVER"
    echo ""
    echo -e "${BLUE}Useful commands:${NC}"
    echo "  Check status:    $INSTALL_DIR/bin/vpn status"
    echo "  View peers:      $INSTALL_DIR/bin/vpn peers"
    echo "  View logs:       sudo cat /var/log/vpn-node.log"
    echo "  Open dashboard:  $INSTALL_DIR/bin/vpn ui"
    echo ""

    if [[ "$OS" == "macos" ]]; then
        echo -e "${BLUE}Service management (macOS):${NC}"
        echo "  Stop:    sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist"
        echo "  Start:   sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist"
    else
        echo -e "${BLUE}Service management (Linux):${NC}"
        echo "  Stop:    sudo systemctl stop vpn-node"
        echo "  Start:   sudo systemctl start vpn-node"
        echo "  Status:  sudo systemctl status vpn-node"
    fi

    echo ""
    echo -e "${GREEN}The VPN will automatically start on boot.${NC}"
    echo ""
}

# Main installation flow
main() {
    print_header "Family VPN Client Installer"

    detect_os
    check_sudo
    install_go
    setup_repository
    build_binaries

    if [[ "$OS" == "macos" ]]; then
        install_macos_service
    else
        install_linux_service
    fi

    start_vpn
    verify_connection
    show_instructions
}

# Run main
main "$@"
