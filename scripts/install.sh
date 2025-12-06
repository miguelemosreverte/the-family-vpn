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

# Sudo password - MUST be loaded from .env file or environment variable
# Never hardcode passwords in committed code!
SUDO_PASSWORD="${SUDO_PASSWORD:-}"

# Load .env file (required for SUDO_PASSWORD)
load_env() {
    local env_file=""

    # Check multiple locations for .env
    if [[ -f "$INSTALL_DIR/.env" ]]; then
        env_file="$INSTALL_DIR/.env"
    elif [[ -f "$(dirname "$0")/../.env" ]]; then
        env_file="$(dirname "$0")/../.env"
    elif [[ -f ".env" ]]; then
        env_file=".env"
    fi

    if [[ -n "$env_file" ]]; then
        print_step "Loading configuration from $env_file"
        # Source the .env file, extracting SUDO_PASSWORD if present
        while IFS='=' read -r key value; do
            # Skip comments and empty lines
            [[ "$key" =~ ^#.*$ ]] && continue
            [[ -z "$key" ]] && continue
            # Remove quotes from value
            value="${value%\"}"
            value="${value#\"}"
            value="${value%\'}"
            value="${value#\'}"
            # Export the variable
            if [[ "$key" == "SUDO_PASSWORD" ]]; then
                SUDO_PASSWORD="$value"
            fi
        done < "$env_file"
    fi

    # Check if SUDO_PASSWORD is set
    if [[ -z "$SUDO_PASSWORD" ]]; then
        print_error "SUDO_PASSWORD not found!"
        print_error "Please create a .env file with SUDO_PASSWORD=yourpassword"
        print_error "Or set the SUDO_PASSWORD environment variable"
        print_error ""
        print_error "You can fetch the .env from the private gist:"
        print_error "  gh gist clone b523442d7bec467dbba22a21feab027e"
        print_error "  cp b523442d7bec467dbba22a21feab027e/.env ."
        exit 1
    fi
}

# Run sudo command with password from SUDO_PASSWORD
run_sudo() {
    echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null
}

# Validate sudo password works and cache credentials
validate_sudo() {
    print_step "Validating sudo access..."
    if echo "$SUDO_PASSWORD" | sudo -S -v 2>/dev/null; then
        print_success "Sudo access validated"
        # Keep sudo timestamp updated in background
        while true; do
            sudo -n true
            sleep 50
            kill -0 "$$" 2>/dev/null || exit
        done &
        return 0
    else
        print_error "Invalid sudo password. Please check SUDO_PASSWORD."
        return 1
    fi
}

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

# Clean up existing installation (for reinstalls)
cleanup_existing() {
    print_step "Cleaning up existing VPN installation..."

    # Kill all VPN processes first
    run_sudo pkill -9 -f "vpn-node" || true
    run_sudo pkill -9 -f "vpn.*ui" || true
    sleep 2

    if [[ "$OSTYPE" == "darwin"* ]]; then
        # Unload all launchd services
        run_sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist || true
        run_sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-ui.plist || true
        run_sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-update.plist || true
        run_sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-health.plist || true

        # Remove plist files
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-node.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-ui.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-update.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-health.plist || true
    else
        # Stop and disable systemd services
        run_sudo systemctl stop vpn-node || true
        run_sudo systemctl stop vpn-ui || true
        run_sudo systemctl stop vpn-update.timer || true
        run_sudo systemctl stop vpn-health.timer || true
        run_sudo systemctl disable vpn-node || true
        run_sudo systemctl disable vpn-ui || true
        run_sudo systemctl disable vpn-update.timer || true
        run_sudo systemctl disable vpn-health.timer || true

        # Remove service files
        run_sudo rm -f /etc/systemd/system/vpn-node.service || true
        run_sudo rm -f /etc/systemd/system/vpn-ui.service || true
        run_sudo rm -f /etc/systemd/system/vpn-update.service || true
        run_sudo rm -f /etc/systemd/system/vpn-update.timer || true
        run_sudo rm -f /etc/systemd/system/vpn-health.service || true
        run_sudo rm -f /etc/systemd/system/vpn-health.timer || true
        run_sudo systemctl daemon-reload || true
    fi

    # Clear health state file (reset failure counter)
    run_sudo rm -f /tmp/vpn-health-state || true
    run_sudo rm -f /tmp/vpn-update.lock || true

    print_success "Cleanup complete"
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

# Install Git if not present (macOS uses Xcode Command Line Tools)
install_git() {
    if command -v git &> /dev/null; then
        print_success "Git is already installed: $(git --version | head -1)"
        return
    fi

    print_step "Installing Git..."

    if [[ "$OS" == "macos" ]]; then
        print_step "Installing Xcode Command Line Tools (includes Git)..."
        print_warning "A dialog may appear asking to install. Click 'Install' and wait."
        echo ""

        # Trigger the Xcode CLI tools installation
        # This will show a GUI dialog on macOS
        xcode-select --install 2>/dev/null || true

        # Wait for the installation to complete
        print_step "Waiting for Xcode Command Line Tools installation..."
        print_warning "Please complete the installation dialog if it appeared."
        echo ""

        # Poll until git becomes available (user completes dialog)
        local max_wait=600  # 10 minutes max
        local waited=0
        while ! command -v git &> /dev/null && [[ $waited -lt $max_wait ]]; do
            sleep 5
            waited=$((waited + 5))
            # Check if xcode-select path is set (means tools are installed)
            if xcode-select -p &> /dev/null; then
                break
            fi
        done

        # Final check
        if command -v git &> /dev/null; then
            print_success "Git installed successfully: $(git --version | head -1)"
        else
            print_error "Git installation timed out or failed!"
            print_error "Please install Xcode Command Line Tools manually:"
            echo "  xcode-select --install"
            echo ""
            echo "Then run this script again."
            exit 1
        fi

    elif [[ "$OS" == "linux" ]]; then
        # Try common package managers
        if command -v apt-get &> /dev/null; then
            print_step "Installing Git via apt..."
            sudo apt-get update
            sudo apt-get install -y git
        elif command -v yum &> /dev/null; then
            print_step "Installing Git via yum..."
            sudo yum install -y git
        elif command -v dnf &> /dev/null; then
            print_step "Installing Git via dnf..."
            sudo dnf install -y git
        elif command -v pacman &> /dev/null; then
            print_step "Installing Git via pacman..."
            sudo pacman -S --noconfirm git
        else
            print_error "Could not detect package manager to install Git!"
            print_error "Please install Git manually and run this script again."
            exit 1
        fi

        if command -v git &> /dev/null; then
            print_success "Git installed successfully: $(git --version | head -1)"
        else
            print_error "Git installation failed!"
            exit 1
        fi
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

# Create the network health watchdog script
create_health_watchdog() {
    print_step "Creating network health watchdog..."

    cat > "$INSTALL_DIR/scripts/health-watchdog.sh" << 'HEALTHSCRIPT'
#!/bin/bash
#
# VPN Network Health Watchdog
# This script runs every minute to ensure internet connectivity.
# If internet fails after VPN starts, it will attempt recovery.
#
# Recovery strategy:
# 1. First 3 failures: Just log and wait
# 2. After 3 consecutive failures: Restart Wi-Fi/network interface
# 3. If that doesn't work: Kill VPN and restart network
#

INSTALL_DIR="$HOME/the-family-vpn"
LOG_FILE="/var/log/vpn-health.log"
STATE_FILE="/tmp/vpn-health-state"
MAX_FAILURES=3

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | sudo tee -a "$LOG_FILE" > /dev/null
}

# Check if we have internet connectivity
check_internet() {
    # Try multiple endpoints to avoid false positives
    # Use IP addresses to avoid DNS issues
    local endpoints=(
        "8.8.8.8"           # Google DNS
        "1.1.1.1"           # Cloudflare DNS
        "95.217.238.72"     # Our VPN server
    )

    for endpoint in "${endpoints[@]}"; do
        if ping -c 1 -W 3 "$endpoint" &>/dev/null; then
            return 0
        fi
    done

    # Also try a TCP connection as backup (in case ICMP is blocked)
    if curl -s --connect-timeout 5 --max-time 10 http://clients3.google.com/generate_204 &>/dev/null; then
        return 0
    fi

    return 1
}

# Get current failure count
get_failure_count() {
    if [[ -f "$STATE_FILE" ]]; then
        cat "$STATE_FILE" 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

# Set failure count
set_failure_count() {
    echo "$1" | sudo tee "$STATE_FILE" > /dev/null
}

# Restart network interface on macOS
restart_network_macos() {
    log "Restarting Wi-Fi on macOS..."

    # Get the Wi-Fi interface name (usually en0)
    local wifi_interface=$(networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}')
    if [[ -z "$wifi_interface" ]]; then
        wifi_interface="en0"
    fi

    log "Wi-Fi interface: $wifi_interface"

    # Turn Wi-Fi off
    networksetup -setairportpower "$wifi_interface" off
    sleep 3

    # Turn Wi-Fi on
    networksetup -setairportpower "$wifi_interface" on
    sleep 5

    log "Wi-Fi restart complete"
}

# Restart network interface on Linux
restart_network_linux() {
    log "Restarting network on Linux..."

    # Try NetworkManager first
    if command -v nmcli &>/dev/null; then
        log "Using NetworkManager..."
        nmcli networking off
        sleep 3
        nmcli networking on
        sleep 5
    # Try systemd-networkd
    elif systemctl is-active systemd-networkd &>/dev/null; then
        log "Using systemd-networkd..."
        sudo systemctl restart systemd-networkd
        sleep 5
    # Fallback: restart the default interface
    else
        log "Using ifconfig fallback..."
        local default_iface=$(ip route | grep default | awk '{print $5}' | head -1)
        if [[ -n "$default_iface" ]]; then
            sudo ifconfig "$default_iface" down
            sleep 3
            sudo ifconfig "$default_iface" up
            sleep 5
        fi
    fi

    log "Network restart complete"
}

# Kill VPN and restart everything
nuclear_option() {
    log "NUCLEAR OPTION: Killing VPN and restarting network..."

    # Kill all VPN processes
    sudo pkill -9 -f "vpn-node" 2>/dev/null || true
    sleep 2

    # Restart network
    if [[ "$OSTYPE" == "darwin"* ]]; then
        restart_network_macos
    else
        restart_network_linux
    fi

    sleep 5

    # Restart VPN service
    log "Restarting VPN service..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist 2>/dev/null || true
    else
        sudo systemctl start vpn-node
    fi

    log "Nuclear recovery complete"
}

# Main health check
main() {
    local failures=$(get_failure_count)

    if check_internet; then
        # Internet is working
        if [[ "$failures" -gt 0 ]]; then
            log "Internet restored after $failures failures"
        fi
        set_failure_count 0
        exit 0
    fi

    # Internet is down
    failures=$((failures + 1))
    set_failure_count "$failures"
    log "Internet check FAILED (failure #$failures)"

    # Check if VPN is even running
    if ! pgrep -f "vpn-node" > /dev/null; then
        log "VPN is not running, skipping network restart (not VPN's fault)"
        exit 0
    fi

    if [[ "$failures" -ge "$MAX_FAILURES" ]]; then
        log "Reached $MAX_FAILURES consecutive failures, attempting recovery..."

        # First try: just restart the network interface
        if [[ "$failures" -eq "$MAX_FAILURES" ]]; then
            log "Attempting network interface restart..."
            if [[ "$OSTYPE" == "darwin"* ]]; then
                restart_network_macos
            else
                restart_network_linux
            fi

            # Check if it worked
            sleep 5
            if check_internet; then
                log "Network restart fixed the issue!"
                set_failure_count 0
                exit 0
            fi
        fi

        # Second try: nuclear option (kill VPN + restart network)
        if [[ "$failures" -ge $((MAX_FAILURES + 2)) ]]; then
            nuclear_option
            set_failure_count 0
        fi
    fi
}

main "$@"
HEALTHSCRIPT

    chmod +x "$INSTALL_DIR/scripts/health-watchdog.sh"
    print_success "Health watchdog created"
}

# Create the periodic update script (runs every 5 minutes during development)
create_update_script() {
    print_step "Creating update script..."

    cat > "$INSTALL_DIR/scripts/update.sh" << 'UPDATESCRIPT'
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
UPDATESCRIPT

    chmod +x "$INSTALL_DIR/scripts/update.sh"
    print_success "Update script created"
}

# Install update job (macOS) - runs every 5 minutes during development
install_macos_update_job() {
    print_step "Installing update job (every 5 minutes)..."

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
        <string>$INSTALL_DIR/scripts/update.sh</string>
    </array>
    <key>StartInterval</key>
    <integer>300</integer>
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

    print_success "Update job installed (every 5 minutes)"
}

# Install health watchdog job (macOS) - runs every minute
install_macos_health_watchdog() {
    print_step "Installing health watchdog (every 60 seconds)..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-health.plist"

    sudo tee "$PLIST_PATH" > /dev/null << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn-health</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>$INSTALL_DIR/scripts/health-watchdog.sh</string>
    </array>
    <key>StartInterval</key>
    <integer>60</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/vpn-health.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/vpn-health.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
EOF

    sudo chown root:wheel "$PLIST_PATH"
    sudo chmod 644 "$PLIST_PATH"

    # Load the job
    sudo launchctl unload "$PLIST_PATH" 2>/dev/null || true
    sudo launchctl load "$PLIST_PATH"

    print_success "Health watchdog installed (checks every 60 seconds)"
}

# Install update job (Linux) - runs every 5 minutes during development
install_linux_update_job() {
    print_step "Installing update job (every 5 minutes)..."

    # Create systemd timer
    sudo tee "/etc/systemd/system/vpn-update.service" > /dev/null << EOF
[Unit]
Description=Family VPN Update Service
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash $INSTALL_DIR/scripts/update.sh
Environment="HOME=$HOME"
Environment="PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/go/bin"
EOF

    sudo tee "/etc/systemd/system/vpn-update.timer" > /dev/null << EOF
[Unit]
Description=Run VPN Update every 5 minutes

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-update.timer
    sudo systemctl start vpn-update.timer

    print_success "Update timer installed (every 5 minutes)"
}

# Install health watchdog job (Linux) - runs every minute
install_linux_health_watchdog() {
    print_step "Installing health watchdog (every 60 seconds)..."

    # Create systemd service
    sudo tee "/etc/systemd/system/vpn-health.service" > /dev/null << EOF
[Unit]
Description=Family VPN Health Watchdog
After=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash $INSTALL_DIR/scripts/health-watchdog.sh
Environment="HOME=$HOME"
EOF

    # Create systemd timer (every minute)
    sudo tee "/etc/systemd/system/vpn-health.timer" > /dev/null << EOF
[Unit]
Description=Run VPN Health Watchdog every minute

[Timer]
OnBootSec=30sec
OnUnitActiveSec=1min
Persistent=true

[Install]
WantedBy=timers.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-health.timer
    sudo systemctl start vpn-health.timer

    print_success "Health watchdog installed (checks every 60 seconds)"
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
    echo "  The VPN will automatically update every 5 minutes"
    echo "  Update logs: sudo cat /var/log/vpn-update.log"
    echo ""
    echo -e "${BLUE}Network Health Watchdog:${NC}"
    echo "  Checks internet connectivity every 60 seconds"
    echo "  Auto-restarts Wi-Fi after 3 consecutive failures"
    echo "  Health logs: sudo cat /var/log/vpn-health.log"
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
    echo -e "${GREEN}The VPN will automatically start on boot, update every 5 min, and self-heal network issues.${NC}"
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
    load_env
    validate_sudo || exit 1
    cleanup_existing
    install_git
    install_go
    setup_repository
    build_binaries
    create_health_watchdog
    create_update_script

    if [[ "$OS" == "macos" ]]; then
        install_macos_service
        install_macos_ui_service
        install_macos_update_job
        install_macos_health_watchdog
    else
        install_linux_service
        install_linux_ui_service
        install_linux_update_job
        install_linux_health_watchdog
    fi

    start_vpn
    verify_connection
    show_instructions
    open_browser
}

# Run main
main "$@"
