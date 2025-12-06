#!/bin/bash
#
# VPN Network Health Watchdog (AGGRESSIVE MODE)
# This script runs as a continuous loop, checking every 5 seconds.
# If internet fails after VPN starts, it will attempt recovery quickly.
#
# Recovery strategy:
# 1. First failure: Just log
# 2. After 2 consecutive failures (10 sec): Restart Wi-Fi/network interface
# 3. After 3 consecutive failures (15 sec): Kill VPN + restart network (nuclear option)
#

INSTALL_DIR="$HOME/the-family-vpn"
LOG_FILE="/var/log/vpn-health.log"
CHECK_INTERVAL=5      # Seconds between checks
WIFI_RESTART_THRESHOLD=2   # Failures before Wi-Fi restart
NUCLEAR_THRESHOLD=3        # Failures before killing VPN

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | sudo tee -a "$LOG_FILE" > /dev/null
}

# Check if we have internet connectivity
check_internet() {
    # Try multiple endpoints to avoid false positives
    # Use IP addresses to avoid DNS issues
    # Short timeout (2 sec) for aggressive checking
    if ping -c 1 -W 2 8.8.8.8 &>/dev/null; then
        return 0
    fi
    if ping -c 1 -W 2 1.1.1.1 &>/dev/null; then
        return 0
    fi
    if ping -c 1 -W 2 95.217.238.72 &>/dev/null; then
        return 0
    fi
    return 1
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
    sleep 2

    # Turn Wi-Fi on
    networksetup -setairportpower "$wifi_interface" on
    sleep 3

    log "Wi-Fi restart complete"
}

# Restart network interface on Linux
restart_network_linux() {
    log "Restarting network on Linux..."

    # Try NetworkManager first
    if command -v nmcli &>/dev/null; then
        log "Using NetworkManager..."
        nmcli networking off
        sleep 2
        nmcli networking on
        sleep 3
    # Try systemd-networkd
    elif systemctl is-active systemd-networkd &>/dev/null; then
        log "Using systemd-networkd..."
        sudo systemctl restart systemd-networkd
        sleep 3
    # Fallback: restart the default interface
    else
        log "Using ifconfig fallback..."
        local default_iface=$(ip route | grep default | awk '{print $5}' | head -1)
        if [[ -n "$default_iface" ]]; then
            sudo ifconfig "$default_iface" down
            sleep 2
            sudo ifconfig "$default_iface" up
            sleep 3
        fi
    fi

    log "Network restart complete"
}

# Restart network based on OS
restart_network() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        restart_network_macos
    else
        restart_network_linux
    fi
}

# Kill VPN and restart everything
nuclear_option() {
    log "NUCLEAR OPTION: Killing VPN and restarting network..."

    # Kill all VPN processes
    sudo pkill -9 -f "vpn-node" 2>/dev/null || true
    sleep 2

    # Restart network
    restart_network
    sleep 3

    # Note: We don't restart VPN here - the install script's cron/launchd will handle that
    log "Nuclear recovery complete - VPN will restart via service manager"
}

# Main watchdog loop
main() {
    log "Health watchdog started (aggressive mode: ${CHECK_INTERVAL}s interval)"
    local failures=0

    while true; do
        if check_internet; then
            # Internet is working
            if [[ "$failures" -gt 0 ]]; then
                log "Internet restored after $failures failure(s)"
            fi
            failures=0
        else
            # Internet is down
            failures=$((failures + 1))
            log "Internet check FAILED (failure #$failures)"

            # Only attempt recovery if VPN is running (otherwise it's not VPN's fault)
            if pgrep -f "vpn-node" > /dev/null; then
                if [[ "$failures" -ge "$NUCLEAR_THRESHOLD" ]]; then
                    # Nuclear option: kill VPN + restart network
                    nuclear_option
                    failures=0
                    sleep 5  # Give network time to stabilize
                elif [[ "$failures" -ge "$WIFI_RESTART_THRESHOLD" ]]; then
                    # First recovery attempt: just restart Wi-Fi
                    log "Attempting network interface restart..."
                    restart_network

                    # Check if it worked
                    sleep 3
                    if check_internet; then
                        log "Network restart fixed the issue!"
                        failures=0
                    fi
                fi
            else
                log "VPN is not running, skipping network restart (not VPN's fault)"
            fi
        fi

        sleep "$CHECK_INTERVAL"
    done
}

# Run if called directly (for testing or background execution)
# If called by launchd/systemd with StartInterval, just run one check
if [[ "$1" == "--once" ]]; then
    # Single check mode (for launchd StartInterval)
    failures=0
    if [[ -f "/tmp/vpn-health-state" ]]; then
        failures=$(cat /tmp/vpn-health-state 2>/dev/null || echo 0)
    fi

    if check_internet; then
        [[ "$failures" -gt 0 ]] && log "Internet restored after $failures failure(s)"
        echo 0 > /tmp/vpn-health-state
    else
        failures=$((failures + 1))
        echo "$failures" > /tmp/vpn-health-state
        log "Internet check FAILED (failure #$failures)"

        if pgrep -f "vpn-node" > /dev/null; then
            if [[ "$failures" -ge "$NUCLEAR_THRESHOLD" ]]; then
                nuclear_option
                echo 0 > /tmp/vpn-health-state
            elif [[ "$failures" -ge "$WIFI_RESTART_THRESHOLD" ]]; then
                log "Attempting network interface restart..."
                restart_network
                sleep 3
                if check_internet; then
                    log "Network restart fixed the issue!"
                    echo 0 > /tmp/vpn-health-state
                fi
            fi
        fi
    fi
else
    # Continuous loop mode (default)
    main "$@"
fi
