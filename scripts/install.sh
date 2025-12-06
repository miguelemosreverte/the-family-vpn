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
GO_VERSION="1.22.0"

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
    # Check if Go is available in common locations
    GO_BIN=""
    if command -v go &> /dev/null; then
        GO_BIN=$(command -v go)
    elif [[ -x "/usr/local/go/bin/go" ]]; then
        GO_BIN="/usr/local/go/bin/go"
        export PATH=$PATH:/usr/local/go/bin
    fi

    if [[ -n "$GO_BIN" ]]; then
        GO_VERSION_INSTALLED=$($GO_BIN version | awk '{print $3}')
        print_success "Go is already installed: $GO_VERSION_INSTALLED"
        return
    fi

    print_step "Installing Go $GO_VERSION..."

    if [[ "$OS" == "macos" ]]; then
        if [[ "$ARCH" == "arm64" ]]; then
            GO_PKG="go${GO_VERSION}.darwin-arm64.pkg"
        else
            GO_PKG="go${GO_VERSION}.darwin-amd64.pkg"
        fi

        print_step "Downloading Go for macOS ($ARCH)..."
        curl -L -o /tmp/go.pkg "https://go.dev/dl/$GO_PKG"

        print_step "Installing Go (requires password)..."
        sudo installer -pkg /tmp/go.pkg -target /
        rm /tmp/go.pkg

        # Add to PATH
        export PATH=$PATH:/usr/local/go/bin

        # Add to shell profiles if not already there
        for profile in ~/.zshrc ~/.bash_profile ~/.profile; do
            if [[ -f "$profile" ]] && ! grep -q '/usr/local/go/bin' "$profile" 2>/dev/null; then
                echo 'export PATH=$PATH:/usr/local/go/bin' >> "$profile"
            fi
        done

    elif [[ "$OS" == "linux" ]]; then
        if [[ "$ARCH" == "aarch64" ]]; then
            GO_TAR="go${GO_VERSION}.linux-arm64.tar.gz"
        else
            GO_TAR="go${GO_VERSION}.linux-amd64.tar.gz"
        fi

        print_step "Downloading Go for Linux ($ARCH)..."
        curl -L -o /tmp/go.tar.gz "https://go.dev/dl/$GO_TAR"

        print_step "Installing Go..."
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz

        # Add to PATH
        export PATH=$PATH:/usr/local/go/bin

        # Add to profile if not already there
        if ! grep -q '/usr/local/go/bin' ~/.profile 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
        fi
        if ! grep -q '/usr/local/go/bin' ~/.bashrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        fi
    fi

    # Verify installation
    if /usr/local/go/bin/go version &> /dev/null; then
        print_success "Go installed successfully: $(/usr/local/go/bin/go version | awk '{print $3}')"
    else
        print_error "Go installation failed!"
        exit 1
    fi
}

# Clone or update repository
setup_repository() {
    if [[ -d "$INSTALL_DIR" ]]; then
        print_step "Updating existing repository..."
        cd "$INSTALL_DIR"
        git fetch origin
        git reset --hard origin/main
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

    # Find Go binary
    GO_CMD="go"
    if ! command -v go &> /dev/null; then
        GO_CMD="/usr/local/go/bin/go"
    fi

    mkdir -p bin
    $GO_CMD build -o bin/vpn-node ./cmd/vpn-node
    $GO_CMD build -o bin/vpn ./cmd/vpn

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

# Create the hourly update script
create_update_script() {
    print_step "Creating hourly update script..."

    cat > "$INSTALL_DIR/scripts/hourly-update.sh" << 'UPDATESCRIPT'
#!/bin/bash
#
# Hourly VPN Update Script
# This script is run every hour to pull updates and restart the VPN if needed.
#

INSTALL_DIR="$HOME/the-family-vpn"
LOG_FILE="/var/log/vpn-update.log"
LOCK_FILE="/tmp/vpn-update.lock"

# Ensure only one instance runs
exec 200>"$LOCK_FILE"
flock -n 200 || exit 0

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
    $GO_CMD build -o bin/vpn-node ./cmd/vpn-node 2>/dev/null
    $GO_CMD build -o bin/vpn ./cmd/vpn 2>/dev/null

    # Sign on macOS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        codesign --sign - --force --deep bin/vpn-node 2>/dev/null
        codesign --sign - --force --deep bin/vpn 2>/dev/null
    fi

    # Restart VPN service
    log "Restarting VPN service..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null
        sleep 1
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist
    else
        sudo systemctl restart vpn-node
    fi

    log "Update complete: $(git log -1 --oneline)"
else
    log "No updates available"
fi

# Ensure VPN is running
if ! pgrep -f "vpn-node" > /dev/null; then
    log "VPN not running, starting..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null
    else
        sudo systemctl start vpn-node
    fi
fi
UPDATESCRIPT

    chmod +x "$INSTALL_DIR/scripts/hourly-update.sh"
    print_success "Update script created"
}

# Install hourly update job (macOS)
install_macos_update_job() {
    print_step "Installing hourly update job..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-update.plist"

    sudo tee "$PLIST_PATH" > /dev/null << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn-update</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>$INSTALL_DIR/scripts/hourly-update.sh</string>
    </array>
    <key>StartInterval</key>
    <integer>3600</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/vpn-update.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/vpn-update.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/go/bin</string>
    </dict>
</dict>
</plist>
EOF

    sudo chown root:wheel "$PLIST_PATH"
    sudo chmod 644 "$PLIST_PATH"

    # Load the job
    sudo launchctl unload "$PLIST_PATH" 2>/dev/null || true
    sudo launchctl load "$PLIST_PATH"

    print_success "Hourly update job installed"
}

# Install hourly update job (Linux)
install_linux_update_job() {
    print_step "Installing hourly update job..."

    # Create systemd timer
    sudo tee "/etc/systemd/system/vpn-update.service" > /dev/null << EOF
[Unit]
Description=Family VPN Update Service
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash $INSTALL_DIR/scripts/hourly-update.sh
Environment="HOME=$HOME"
Environment="PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/go/bin"
EOF

    sudo tee "/etc/systemd/system/vpn-update.timer" > /dev/null << EOF
[Unit]
Description=Run VPN Update every hour

[Timer]
OnBootSec=5min
OnUnitActiveSec=1h
Persistent=true

[Install]
WantedBy=timers.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-update.timer
    sudo systemctl start vpn-update.timer

    print_success "Hourly update timer installed"
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

# Install UI service (macOS) - runs the web dashboard
install_macos_ui_service() {
    print_step "Installing UI service for web dashboard..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-ui.plist"

    # Create plist for UI
    sudo tee "$PLIST_PATH" > /dev/null << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn-ui</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/bin/vpn</string>
        <string>--node</string>
        <string>95.217.238.72:9001</string>
        <string>ui</string>
        <string>--listen</string>
        <string>localhost:8080</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/vpn-ui.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/vpn-ui.log</string>
    <key>WorkingDirectory</key>
    <string>$INSTALL_DIR</string>
</dict>
</plist>
EOF

    sudo chown root:wheel "$PLIST_PATH"
    sudo chmod 644 "$PLIST_PATH"

    # Load the UI service
    sudo launchctl unload "$PLIST_PATH" 2>/dev/null || true
    sudo launchctl load "$PLIST_PATH"

    print_success "UI service installed (http://localhost:8080)"
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

# Install UI service (Linux) - runs the web dashboard
install_linux_ui_service() {
    print_step "Installing UI service for web dashboard..."

    sudo tee "/etc/systemd/system/vpn-ui.service" > /dev/null << EOF
[Unit]
Description=Family VPN Web Dashboard
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/vpn --node 95.217.238.72:9001 ui --listen localhost:8080
Restart=always
RestartSec=5
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-ui
    sudo systemctl start vpn-ui

    print_success "UI service installed (http://localhost:8080)"
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
    echo -e "${BLUE}Automatic Updates:${NC}"
    echo "  The VPN will automatically update every hour"
    echo "  Update logs: sudo cat /var/log/vpn-update.log"
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
    echo -e "${GREEN}The VPN will automatically start on boot and update hourly.${NC}"
    echo ""
}

# Open browser to dashboard
open_browser() {
    print_step "Opening VPN dashboard in browser..."
    sleep 2  # Give UI time to start

    if [[ "$OS" == "macos" ]]; then
        open "http://localhost:8080" 2>/dev/null || true
    else
        # Try various Linux browsers
        xdg-open "http://localhost:8080" 2>/dev/null || \
        sensible-browser "http://localhost:8080" 2>/dev/null || \
        firefox "http://localhost:8080" 2>/dev/null || \
        chromium "http://localhost:8080" 2>/dev/null || \
        google-chrome "http://localhost:8080" 2>/dev/null || true
    fi

    print_success "Dashboard available at http://localhost:8080"
}

# Main installation flow
main() {
    print_header "Family VPN Client Installer"

    detect_os
    check_sudo
    install_go
    setup_repository
    build_binaries
    create_update_script

    if [[ "$OS" == "macos" ]]; then
        install_macos_service
        install_macos_ui_service
        install_macos_update_job
    else
        install_linux_service
        install_linux_ui_service
        install_linux_update_job
    fi

    start_vpn
    verify_connection
    show_instructions
    open_browser
}

# Run main
main "$@"
