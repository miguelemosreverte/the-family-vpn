#!/bin/bash
#
# macOS Sharing Setup Wizard
# Checks and enables necessary sharing settings for VPN remote access
#
# Settings configured:
#   1. Remote Login (SSH) - for terminal access
#   2. Screen Sharing (VNC) - for remote desktop access
#   3. File Sharing (optional) - for file transfers
#
# Usage: ./scripts/setup-sharing.sh
#

INSTALL_DIR="$HOME/the-family-vpn"

# Load .env if it exists (for SUDO_PASSWORD)
if [[ -f "$INSTALL_DIR/.env" ]]; then
    source "$INSTALL_DIR/.env"
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Helper function to run sudo with password from .env
run_sudo() {
    if [[ -n "$SUDO_PASSWORD" ]]; then
        echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null
    else
        sudo "$@"
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

print_success() {
    echo -e "${GREEN}✔${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✖${NC} $1"
}

print_info() {
    echo -e "${CYAN}ℹ${NC} $1"
}

# Check if running on macOS
check_macos() {
    if [[ "$OSTYPE" != "darwin"* ]]; then
        print_error "This script is only for macOS"
        exit 1
    fi
}

# Check Remote Login (SSH) status
check_ssh() {
    local status
    status=$(run_sudo systemsetup -getremotelogin 2>&1 | grep -i "on")
    if [[ -n "$status" ]]; then
        return 0  # Enabled
    else
        return 1  # Disabled
    fi
}

# Enable Remote Login (SSH)
enable_ssh() {
    print_step "Enabling Remote Login (SSH)..."

    # Enable SSH via systemsetup
    run_sudo systemsetup -setremotelogin on 2>/dev/null

    # Also ensure the launchd service is loaded
    run_sudo launchctl load -w /System/Library/LaunchDaemons/ssh.plist 2>/dev/null

    # Verify
    if check_ssh; then
        print_success "Remote Login (SSH) enabled"
        return 0
    else
        return 1  # Return failure, caller will handle
    fi
}

# Enable SSH with fallback to GUI
enable_ssh_with_fallback() {
    if enable_ssh; then
        return 0
    fi

    # Programmatic enable failed - need user to enable in System Settings
    echo ""
    print_warning "Remote Login (SSH) requires manual approval"
    print_info "macOS security requires you to enable this in System Settings"
    echo ""
    echo -e "  ${CYAN}Please toggle ON 'Remote Login' in the window that opens${NC}"
    echo ""

    if [[ -t 0 ]]; then
        # Interactive mode - open settings and wait
        open_sharing_settings
        read -p "Press Enter after enabling Remote Login in System Settings... "

        # Re-check
        sleep 1
        if check_ssh; then
            print_success "Remote Login (SSH) enabled"
            return 0
        else
            print_error "Remote Login still not enabled"
            return 1
        fi
    else
        # Non-interactive mode - just inform
        print_warning "Run this script interactively to enable Remote Login"
        return 1
    fi
}

# Check Screen Sharing status
check_screen_sharing() {
    # Check if screensharing service is loaded
    if run_sudo launchctl list 2>/dev/null | grep -q "com.apple.screensharing"; then
        # Check if it's actually running (not just loaded)
        local status
        status=$(run_sudo launchctl list com.apple.screensharing 2>/dev/null | grep -c "PID")
        if [[ "$status" -gt 0 ]] || netstat -an 2>/dev/null | grep -q "\.5900 .*LISTEN"; then
            return 0  # Enabled and running
        fi
    fi
    return 1  # Disabled
}

# Open System Settings to Sharing pane
open_sharing_settings() {
    print_info "Opening System Settings > Sharing..."
    # macOS Ventura+ uses different URL scheme
    if [[ $(sw_vers -productVersion | cut -d. -f1) -ge 13 ]]; then
        open "x-apple.systempreferences:com.apple.Sharing-Settings.extension"
    else
        open "x-apple.systempreferences:com.apple.preference.sharing"
    fi
}

# Enable Screen Sharing
enable_screen_sharing() {
    print_step "Enabling Screen Sharing..."

    # Method 1: Using kickstart (most reliable)
    local kickstart="/System/Library/CoreServices/RemoteManagement/ARDAgent.app/Contents/Resources/kickstart"

    if [[ -x "$kickstart" ]]; then
        # Enable and activate Remote Management with VNC
        run_sudo "$kickstart" -activate -configure -access -on -restart -agent -privs -all 2>/dev/null

        # Enable VNC with password from .env
        if [[ -n "$VNC_PASSWORD" ]]; then
            run_sudo "$kickstart" -configure -clientopts -setvnclegacy -vnclegacy yes 2>/dev/null
            run_sudo "$kickstart" -configure -clientopts -setvncpw -vncpw "$VNC_PASSWORD" 2>/dev/null
        fi
    fi

    # Method 2: Direct launchctl (fallback)
    run_sudo launchctl load -w /System/Library/LaunchDaemons/com.apple.screensharing.plist 2>/dev/null

    # Method 3: Using defaults (for newer macOS)
    run_sudo defaults write /var/db/launchd.db/com.apple.launchd/overrides.plist com.apple.screensharing -dict Disabled -bool false 2>/dev/null

    sleep 2

    # Verify
    if check_screen_sharing || netstat -an 2>/dev/null | grep -q "\.5900 .*LISTEN"; then
        print_success "Screen Sharing enabled"
        return 0
    else
        return 1  # Return failure, caller will handle manual enable
    fi
}

# Enable Screen Sharing with fallback to GUI
enable_screen_sharing_with_fallback() {
    if enable_screen_sharing; then
        return 0
    fi

    # Programmatic enable failed - need user to click in System Settings
    echo ""
    print_warning "Screen Sharing requires manual approval on modern macOS"
    print_info "macOS security requires you to enable this in System Settings"
    echo ""
    echo -e "  ${CYAN}Please toggle ON 'Screen Sharing' in the window that opens${NC}"
    echo ""

    if [[ -t 0 ]]; then
        # Interactive mode - open settings and wait
        open_sharing_settings
        read -p "Press Enter after enabling Screen Sharing in System Settings... "

        # Re-check
        sleep 1
        if check_screen_sharing; then
            print_success "Screen Sharing enabled"
            return 0
        else
            print_error "Screen Sharing still not enabled"
            return 1
        fi
    else
        # Non-interactive mode - just inform
        print_warning "Run this script interactively to enable Screen Sharing"
        return 1
    fi
}

# Check File Sharing status
check_file_sharing() {
    if run_sudo launchctl list 2>/dev/null | grep -q "com.apple.smbd"; then
        return 0
    fi
    return 1
}

# Enable File Sharing
enable_file_sharing() {
    print_step "Enabling File Sharing..."

    run_sudo launchctl load -w /System/Library/LaunchDaemons/com.apple.smbd.plist 2>/dev/null

    if check_file_sharing; then
        print_success "File Sharing enabled"
        return 0
    else
        print_warning "File Sharing may require manual enabling"
        return 1
    fi
}

# Display current status
show_status() {
    print_header "Current Sharing Status"

    echo "Checking sharing settings..."
    echo ""

    # SSH
    if check_ssh; then
        echo -e "  [${GREEN}✔${NC}] Remote Login (SSH)     - ${GREEN}Enabled${NC}"
    else
        echo -e "  [${RED}✖${NC}] Remote Login (SSH)     - ${RED}Disabled${NC}"
    fi

    # Screen Sharing
    if check_screen_sharing; then
        echo -e "  [${GREEN}✔${NC}] Screen Sharing (VNC)   - ${GREEN}Enabled${NC}"
    else
        echo -e "  [${RED}✖${NC}] Screen Sharing (VNC)   - ${RED}Disabled${NC}"
    fi

    # File Sharing
    if check_file_sharing; then
        echo -e "  [${GREEN}✔${NC}] File Sharing (SMB)     - ${GREEN}Enabled${NC}"
    else
        echo -e "  [${YELLOW}-${NC}] File Sharing (SMB)     - ${YELLOW}Disabled${NC} (optional)"
    fi

    echo ""
}

# Interactive wizard
run_wizard() {
    print_header "macOS Sharing Setup Wizard"

    echo "This wizard will configure your Mac for remote access via VPN."
    echo "The following settings will be checked and enabled if needed:"
    echo ""
    echo "  1. Remote Login (SSH)   - Terminal access over network"
    echo "  2. Screen Sharing       - Remote desktop via VNC"
    echo "  3. File Sharing         - Access files remotely (optional)"
    echo ""

    # Show current status
    show_status

    # Check what needs to be enabled
    local needs_ssh=false
    local needs_screen=false

    if ! check_ssh; then
        needs_ssh=true
    fi

    if ! check_screen_sharing; then
        needs_screen=true
    fi

    if [[ "$needs_ssh" == "false" && "$needs_screen" == "false" ]]; then
        print_success "All required sharing settings are already enabled!"
        return 0
    fi

    echo ""
    echo "The following settings need to be enabled:"
    [[ "$needs_ssh" == "true" ]] && echo "  - Remote Login (SSH)"
    [[ "$needs_screen" == "true" ]] && echo "  - Screen Sharing"
    echo ""

    # Auto-enable if running non-interactively or with --auto flag
    if [[ "$1" == "--auto" ]] || [[ ! -t 0 ]]; then
        print_step "Auto-enabling required settings..."
    else
        read -p "Enable these settings now? [Y/n] " response
        if [[ "$response" =~ ^[Nn] ]]; then
            print_warning "Skipping sharing setup. You can run this wizard later with:"
            echo "  $INSTALL_DIR/scripts/setup-sharing.sh"
            return 1
        fi
    fi

    # Enable SSH if needed (with GUI fallback)
    if [[ "$needs_ssh" == "true" ]]; then
        enable_ssh_with_fallback
    fi

    # Enable Screen Sharing if needed (with GUI fallback)
    if [[ "$needs_screen" == "true" ]]; then
        enable_screen_sharing_with_fallback
    fi

    echo ""
    print_header "Setup Complete"

    # Show final status
    show_status

    # Show connection info
    local hostname=$(scutil --get LocalHostName 2>/dev/null || hostname -s)
    local vpn_ip=$(ifconfig 2>/dev/null | grep -A1 "utun" | grep "inet " | awk '{print $2}' | head -1)

    echo "Other VPN users can now connect to this Mac:"
    echo ""
    if [[ -n "$vpn_ip" ]]; then
        echo "  SSH:    ssh $USER@$vpn_ip"
        echo "  VNC:    vnc://$vpn_ip"
    else
        echo "  SSH:    ssh $USER@$hostname.local"
        echo "  VNC:    vnc://$hostname.local"
    fi
    echo ""
}

# Main
check_macos

case "$1" in
    --status)
        show_status
        ;;
    --enable-ssh)
        enable_ssh
        ;;
    --enable-screen)
        enable_screen_sharing
        ;;
    --enable-files)
        enable_file_sharing
        ;;
    --auto)
        run_wizard --auto
        ;;
    *)
        run_wizard "$@"
        ;;
esac
