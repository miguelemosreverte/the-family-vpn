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

# Update job configuration
# Change this value to adjust how often the VPN checks for updates (in seconds)
UPDATE_INTERVAL_SECONDS=300  # 5 minutes

# Sudo password - MUST be loaded from .env file or environment variable
# Never hardcode passwords in committed code!
SUDO_PASSWORD="${SUDO_PASSWORD:-}"

# Load .env file (required for SUDO_PASSWORD)
# The .env file is included in the repository for convenience
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
        print_error "The .env file should be included in the repository."
        print_error "Try: git pull origin main"
        exit 1
    fi
}

# Run sudo command with password from SUDO_PASSWORD
run_sudo() {
    # Use sudo -n first (non-interactive, uses cached credentials)
    # If that fails, fall back to password
    if sudo -n "$@" 2>/dev/null; then
        return 0
    fi
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

    # First, gracefully disconnect VPN to restore routing table
    # This is critical - killing vpn-node without disconnect leaves routes broken
    if [[ -x "$INSTALL_DIR/bin/vpn" ]]; then
        print_step "Gracefully disconnecting VPN..."
        "$INSTALL_DIR/bin/vpn" disconnect 2>/dev/null || true
        sleep 2
    fi

    # Kill UI processes (these don't affect routing)
    run_sudo pkill -9 -f "vpn.*ui" || true

    if [[ "$OSTYPE" == "darwin"* ]]; then
        # Save the current default gateway before stopping VPN
        # In case graceful disconnect didn't work
        CURRENT_GW=$(route -n get default 2>/dev/null | grep gateway | awk '{print $2}')
        CURRENT_IF=$(route -n get default 2>/dev/null | grep interface | awk '{print $2}')

        # Unload all launchd services (this will cleanly stop vpn-node)
        run_sudo launchctl bootout system/com.family.vpn-node 2>/dev/null || true
        run_sudo launchctl bootout system/com.family.vpn-ui 2>/dev/null || true
        run_sudo launchctl bootout system/com.family.vpn-update 2>/dev/null || true
        run_sudo launchctl bootout system/com.family.vpn-health 2>/dev/null || true
        run_sudo launchctl bootout system/com.family.vpn-nosleep 2>/dev/null || true
        sleep 2

        # Kill any remaining VPN processes
        run_sudo pkill -9 -f "vpn-node" || true
        sleep 1

        # Restore default route if it was lost
        if ! route -n get default &>/dev/null; then
            print_warning "Default route lost, attempting to restore..."
            # Try to get gateway from network service
            for SERVICE in "Wi-Fi" "Ethernet" "USB 10/100/1000 LAN"; do
                GW=$(networksetup -getinfo "$SERVICE" 2>/dev/null | grep "^Router:" | awk '{print $2}')
                if [[ -n "$GW" && "$GW" != "none" ]]; then
                    run_sudo route add default "$GW" 2>/dev/null || true
                    print_success "Restored default route via $SERVICE ($GW)"
                    break
                fi
            done
        fi

        # Remove plist files
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-node.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-ui.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-update.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-health.plist || true
        run_sudo rm -f /Library/LaunchDaemons/com.family.vpn-nosleep.plist || true
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

        # Kill any remaining VPN processes
        run_sudo pkill -9 -f "vpn-node" || true

        # Remove service files
        run_sudo rm -f /etc/systemd/system/vpn-node.service || true
        run_sudo rm -f /etc/systemd/system/vpn-ui.service || true
        run_sudo rm -f /etc/systemd/system/vpn-update.service || true
        run_sudo rm -f /etc/systemd/system/vpn-update.timer || true
        run_sudo rm -f /etc/systemd/system/vpn-health.service || true
        run_sudo rm -f /etc/systemd/system/vpn-health.timer || true
        run_sudo systemctl daemon-reload || true
    fi

    # Verify network connectivity before proceeding
    print_step "Verifying network connectivity..."
    if ! curl -s --max-time 5 https://github.com &>/dev/null; then
        print_warning "Network connectivity issue detected, waiting for recovery..."
        sleep 3
        # Try to ping google DNS as fallback check
        if ! ping -c 1 -W 2 8.8.8.8 &>/dev/null; then
            print_error "Network still not available. You may need to restart Wi-Fi."
        fi
    fi

    # Clear state files (reset failure counter, locks)
    run_sudo rm -f /tmp/vpn-health-state || true
    run_sudo rm -f /tmp/vpn-update.lock || true
    run_sudo rm -rf /tmp/vpn-update.lock || true

    # Clear old log files for fresh start
    run_sudo rm -f /var/log/vpn-node.log || true
    run_sudo rm -f /var/log/vpn-ui.log || true
    run_sudo rm -f /var/log/vpn-update.log || true
    run_sudo rm -f /var/log/vpn-health.log || true

    # Clear old binaries (will be rebuilt)
    rm -f "$INSTALL_DIR/bin/vpn-node" 2>/dev/null || true
    rm -f "$INSTALL_DIR/bin/vpn" 2>/dev/null || true

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

# Install sshpass (macOS) - required for UI SSH functionality
install_sshpass_macos() {
    if command -v sshpass &> /dev/null || [[ -x "/opt/homebrew/bin/sshpass" ]]; then
        print_success "sshpass is already installed"
        return
    fi

    print_step "Installing sshpass for SSH functionality..."

    # Check if Homebrew is installed
    if ! command -v brew &> /dev/null; then
        if [[ -x "/opt/homebrew/bin/brew" ]]; then
            export PATH="/opt/homebrew/bin:$PATH"
        elif [[ -x "/usr/local/bin/brew" ]]; then
            export PATH="/usr/local/bin:$PATH"
        else
            print_warning "Homebrew not found. Installing Homebrew first..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            if [[ -x "/opt/homebrew/bin/brew" ]]; then
                export PATH="/opt/homebrew/bin:$PATH"
            fi
        fi
    fi

    # Install sshpass via Homebrew
    # Note: sshpass is in a tap, not the main repo
    brew install hudochenkov/sshpass/sshpass 2>/dev/null || brew install sshpass 2>/dev/null || true

    if command -v sshpass &> /dev/null || [[ -x "/opt/homebrew/bin/sshpass" ]]; then
        print_success "sshpass installed successfully"
    else
        print_warning "sshpass installation failed - SSH from UI may not work"
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

    # Get git version for embedding
    GIT_VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
    LDFLAGS="-X github.com/family-vpn/the-family-vpn/internal/node.Version=$GIT_VERSION"

    $GO_CMD build -ldflags "$LDFLAGS" -o bin/vpn-node ./cmd/vpn-node
    $GO_CMD build -ldflags "$LDFLAGS" -o bin/vpn ./cmd/vpn

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

# Ensure the health watchdog script is executable
# The script is already in the repository at scripts/health-watchdog.sh
setup_health_watchdog() {
    print_step "Setting up network health watchdog..."
    chmod +x "$INSTALL_DIR/scripts/health-watchdog.sh"
    print_success "Health watchdog ready (aggressive mode: 5s checks)"
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

    # Rebuild binaries with version embedded
    log "Rebuilding binaries..."
    GIT_VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
    LDFLAGS="-X github.com/family-vpn/the-family-vpn/internal/node.Version=$GIT_VERSION"
    $GO_CMD build -ldflags "$LDFLAGS" -o bin/vpn-node ./cmd/vpn-node 2>&1 | sudo tee -a "$LOG_FILE"
    $GO_CMD build -ldflags "$LDFLAGS" -o bin/vpn ./cmd/vpn 2>&1 | sudo tee -a "$LOG_FILE"

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

# Install update job (macOS) - idempotent, only reloads if config changed
install_macos_update_job() {
    local interval_minutes=$((UPDATE_INTERVAL_SECONDS / 60))
    print_step "Installing update job (every ${interval_minutes} minutes)..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-update.plist"

    # Generate the desired plist content
    local NEW_PLIST
    NEW_PLIST=$(cat << EOF
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
    <integer>$UPDATE_INTERVAL_SECONDS</integer>
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
)

    # Check if plist exists and compare content
    local NEEDS_UPDATE=true
    if [[ -f "$PLIST_PATH" ]]; then
        local CURRENT_PLIST
        CURRENT_PLIST=$(sudo cat "$PLIST_PATH" 2>/dev/null)
        if [[ "$CURRENT_PLIST" == "$NEW_PLIST" ]]; then
            NEEDS_UPDATE=false
            print_success "Update job already configured (every ${interval_minutes} minutes) - no changes needed"
        fi
    fi

    if [[ "$NEEDS_UPDATE" == "true" ]]; then
        echo "$NEW_PLIST" | sudo tee "$PLIST_PATH" > /dev/null
        sudo chown root:wheel "$PLIST_PATH"
        sudo chmod 644 "$PLIST_PATH"

        # Reload the job only if it changed
        sudo launchctl bootout system/com.family.vpn-update 2>/dev/null || true
        sudo launchctl bootstrap system "$PLIST_PATH"

        print_success "Update job installed (every ${interval_minutes} minutes)"
    fi
}

# Install health watchdog job (macOS) - idempotent, only reloads if config changed
install_macos_health_watchdog() {
    print_step "Installing health watchdog (aggressive: 5 second checks)..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-health.plist"

    # Generate the desired plist content
    # The health-watchdog.sh runs as a continuous loop, so we use KeepAlive instead of StartInterval
    local NEW_PLIST
    NEW_PLIST=$(cat << EOF
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
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
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
)

    # Check if plist exists and compare content
    local NEEDS_UPDATE=true
    if [[ -f "$PLIST_PATH" ]]; then
        local CURRENT_PLIST
        CURRENT_PLIST=$(sudo cat "$PLIST_PATH" 2>/dev/null)
        if [[ "$CURRENT_PLIST" == "$NEW_PLIST" ]]; then
            NEEDS_UPDATE=false
            print_success "Health watchdog already configured - no changes needed"
        fi
    fi

    if [[ "$NEEDS_UPDATE" == "true" ]]; then
        echo "$NEW_PLIST" | sudo tee "$PLIST_PATH" > /dev/null
        sudo chown root:wheel "$PLIST_PATH"
        sudo chmod 644 "$PLIST_PATH"

        # Reload the job only if it changed
        sudo launchctl bootout system/com.family.vpn-health 2>/dev/null || true
        sudo launchctl bootstrap system "$PLIST_PATH"

        print_success "Health watchdog installed (aggressive: checks every 5 seconds)"
    fi
}

# Install update job (Linux) - idempotent, only reloads if config changed
install_linux_update_job() {
    local interval_minutes=$((UPDATE_INTERVAL_SECONDS / 60))
    print_step "Installing update job (every ${interval_minutes} minutes)..."

    SERVICE_PATH="/etc/systemd/system/vpn-update.service"
    TIMER_PATH="/etc/systemd/system/vpn-update.timer"

    # Generate the desired service content
    local NEW_SERVICE
    NEW_SERVICE=$(cat << EOF
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
)

    # Generate the desired timer content
    local NEW_TIMER
    NEW_TIMER=$(cat << EOF
[Unit]
Description=Run VPN Update every ${interval_minutes} minutes

[Timer]
OnBootSec=1min
OnUnitActiveSec=${interval_minutes}min
Persistent=true

[Install]
WantedBy=timers.target
EOF
)

    # Check if files exist and compare content
    local NEEDS_UPDATE=false
    if [[ -f "$SERVICE_PATH" ]] && [[ -f "$TIMER_PATH" ]]; then
        local CURRENT_SERVICE
        local CURRENT_TIMER
        CURRENT_SERVICE=$(sudo cat "$SERVICE_PATH" 2>/dev/null)
        CURRENT_TIMER=$(sudo cat "$TIMER_PATH" 2>/dev/null)
        if [[ "$CURRENT_SERVICE" != "$NEW_SERVICE" ]] || [[ "$CURRENT_TIMER" != "$NEW_TIMER" ]]; then
            NEEDS_UPDATE=true
        fi
    else
        NEEDS_UPDATE=true
    fi

    if [[ "$NEEDS_UPDATE" == "true" ]]; then
        echo "$NEW_SERVICE" | sudo tee "$SERVICE_PATH" > /dev/null
        echo "$NEW_TIMER" | sudo tee "$TIMER_PATH" > /dev/null

        sudo systemctl daemon-reload
        sudo systemctl enable vpn-update.timer
        sudo systemctl restart vpn-update.timer

        print_success "Update timer installed (every ${interval_minutes} minutes)"
    else
        print_success "Update timer already configured (every ${interval_minutes} minutes) - no changes needed"
    fi
}

# Install health watchdog job (Linux) - idempotent, only reloads if config changed
install_linux_health_watchdog() {
    print_step "Installing health watchdog (aggressive: 5 second checks)..."

    SERVICE_PATH="/etc/systemd/system/vpn-health.service"

    # Generate the desired service content
    local NEW_SERVICE
    NEW_SERVICE=$(cat << EOF
[Unit]
Description=Family VPN Health Watchdog (Aggressive)
After=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash $INSTALL_DIR/scripts/health-watchdog.sh
Environment="HOME=$HOME"
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
)

    # Check if file exists and compare content
    local NEEDS_UPDATE=false
    if [[ -f "$SERVICE_PATH" ]]; then
        local CURRENT_SERVICE
        CURRENT_SERVICE=$(sudo cat "$SERVICE_PATH" 2>/dev/null)
        if [[ "$CURRENT_SERVICE" != "$NEW_SERVICE" ]]; then
            NEEDS_UPDATE=true
        fi
    else
        NEEDS_UPDATE=true
    fi

    # Always clean up old timer if it exists (legacy)
    if systemctl is-active vpn-health.timer &>/dev/null || [[ -f /etc/systemd/system/vpn-health.timer ]]; then
        sudo systemctl stop vpn-health.timer 2>/dev/null || true
        sudo systemctl disable vpn-health.timer 2>/dev/null || true
        sudo rm -f /etc/systemd/system/vpn-health.timer
        NEEDS_UPDATE=true  # Force reload after timer cleanup
    fi

    if [[ "$NEEDS_UPDATE" == "true" ]]; then
        echo "$NEW_SERVICE" | sudo tee "$SERVICE_PATH" > /dev/null

        sudo systemctl daemon-reload
        sudo systemctl enable vpn-health.service
        sudo systemctl restart vpn-health.service

        print_success "Health watchdog installed (aggressive: checks every 5 seconds)"
    else
        print_success "Health watchdog already configured - no changes needed"
    fi
}

# Configure sleep prevention (macOS) - keeps computer awake for VPN
configure_macos_sleep_prevention() {
    print_step "Configuring sleep prevention (keeps computer awake for VPN)..."

    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-nosleep.plist"

    # Generate the desired plist content
    # caffeinate -s prevents system sleep indefinitely (runs forever with -i -d -s)
    # -i: prevent idle sleep
    # -d: prevent display sleep (optional, remove if you want display to sleep)
    # -s: prevent system sleep when on AC power
    local NEW_PLIST
    NEW_PLIST=$(cat << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn-nosleep</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/caffeinate</string>
        <string>-i</string>
        <string>-s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF
)

    # Check if plist exists and compare content
    local NEEDS_UPDATE=true
    if [[ -f "$PLIST_PATH" ]]; then
        local CURRENT_PLIST
        CURRENT_PLIST=$(sudo cat "$PLIST_PATH" 2>/dev/null)
        if [[ "$CURRENT_PLIST" == "$NEW_PLIST" ]]; then
            NEEDS_UPDATE=false
            print_success "Sleep prevention already configured - no changes needed"
        fi
    fi

    if [[ "$NEEDS_UPDATE" == "true" ]]; then
        echo "$NEW_PLIST" | sudo tee "$PLIST_PATH" > /dev/null
        sudo chown root:wheel "$PLIST_PATH"
        sudo chmod 644 "$PLIST_PATH"

        # Reload the job only if it changed
        sudo launchctl bootout system/com.family.vpn-nosleep 2>/dev/null || true
        sudo launchctl bootstrap system "$PLIST_PATH"

        print_success "Sleep prevention installed (computer will stay awake)"
    fi
}

# Install launchd service (macOS)
install_macos_service() {
    print_step "Installing launchd service for auto-start..."

    NODE_NAME=$(get_node_name)
    PLIST_PATH="/Library/LaunchDaemons/com.family.vpn-node.plist"

    # Create plist
    # IMPORTANT: PATH includes /opt/homebrew/bin for sshpass (used for SSH from UI terminal)
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
        <string>--route-all</string>
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
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
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
    # IMPORTANT: PATH includes /opt/homebrew/bin for sshpass (used for SSH from UI)
    # Uses --templates for hot reload (changes take effect on browser refresh)
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
        <string>--templates</string>
        <string>$INSTALL_DIR/internal/ui/templates</string>
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
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
EOF

    sudo chown root:wheel "$PLIST_PATH"
    sudo chmod 644 "$PLIST_PATH"

    # Kill any existing UI processes to ensure clean restart with new binary
    run_sudo pkill -9 -f "vpn.*ui" || true
    sleep 1

    # Unload and reload the UI service to pick up new binary
    sudo launchctl bootout system/com.family.vpn-ui 2>/dev/null || true
    sleep 1
    sudo launchctl bootstrap system "$PLIST_PATH"

    print_success "UI service installed and restarted (http://localhost:8080)"
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
ExecStart=$INSTALL_DIR/bin/vpn --node 95.217.238.72:9001 ui --listen localhost:8080 --templates $INSTALL_DIR/internal/ui/templates
Restart=always
RestartSec=5
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

    # Kill any existing UI processes to ensure clean restart with new binary
    sudo pkill -9 -f "vpn.*ui" 2>/dev/null || true
    sleep 1

    sudo systemctl daemon-reload
    sudo systemctl enable vpn-ui
    sudo systemctl restart vpn-ui

    print_success "UI service installed and restarted (http://localhost:8080)"
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
    echo -e "${BLUE}Network Health Watchdog (Aggressive Mode):${NC}"
    echo "  Checks internet connectivity every 5 seconds"
    echo "  Auto-restarts Wi-Fi after 2 consecutive failures (10 sec)"
    echo "  Kills VPN + restarts Wi-Fi after 3 failures (15 sec)"
    echo "  Health logs: sudo cat /var/log/vpn-health.log"
    echo ""
    if [[ "$OS" == "macos" ]]; then
        echo -e "${BLUE}Sleep Prevention (macOS):${NC}"
        echo "  Computer will stay awake to maintain VPN connection"
        echo "  Display may still sleep, but system won't"
        echo ""
    fi
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

# Send install handshake to server
send_handshake() {
    print_step "Sending install handshake to server..."

    cd "$INSTALL_DIR"

    # Get git commit hash for version
    GIT_VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

    # Wait a moment for VPN to be fully connected
    sleep 2

    # Send handshake directly to server via VPN tunnel (10.8.0.1:9001 is the server's control port)
    # This ensures handshakes are stored centrally on the server, not on the local client
    # We pass --name explicitly to identify the client, since connecting to server gets server's status
    if ./bin/vpn handshake --version "$GIT_VERSION" --name "$NODE_NAME" --node 10.8.0.1:9001 2>/dev/null; then
        print_success "Handshake sent successfully"
    else
        print_warning "Handshake may have failed (server might record it anyway)"
    fi
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
    if [[ "$OS" == "macos" ]]; then
        install_sshpass_macos
    fi
    setup_repository
    build_binaries
    setup_health_watchdog
    create_update_script

    if [[ "$OS" == "macos" ]]; then
        install_macos_service
        install_macos_ui_service
        install_macos_update_job
        install_macos_health_watchdog
        configure_macos_sleep_prevention
    else
        install_linux_service
        install_linux_ui_service
        install_linux_update_job
        install_linux_health_watchdog
    fi

    start_vpn
    verify_connection
    send_handshake

    # Configure macOS sharing settings (SSH, Screen Sharing)
    if [[ "$OS" == "macos" ]]; then
        setup_macos_sharing
    fi

    show_instructions
    run_validation
    open_browser
}

# Configure macOS sharing settings (SSH, Screen Sharing)
setup_macos_sharing() {
    print_step "Checking macOS sharing settings..."

    if [[ -x "$INSTALL_DIR/scripts/setup-sharing.sh" ]]; then
        # Run the sharing setup wizard in auto mode
        "$INSTALL_DIR/scripts/setup-sharing.sh" --auto
    else
        print_warning "Sharing setup script not found, skipping..."
    fi
}

# Run system health validation and save report
run_validation() {
    print_step "Running system health validation..."

    # Give services a moment to stabilize
    sleep 3

    if [[ -x "$INSTALL_DIR/scripts/validate.sh" ]]; then
        echo ""
        echo "════════════════════════════════════════════════════════════════"
        "$INSTALL_DIR/scripts/validate.sh"
        echo "════════════════════════════════════════════════════════════════"
        echo ""
        print_success "Health report saved to $INSTALL_DIR/HEALTH_REPORT.md"
    else
        print_warning "Validation script not found"
    fi
}

# Run main
main "$@"
