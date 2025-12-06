# Family VPN - Installation Guide

This guide explains how to set up the Family VPN client on a new computer.

## Quick Start (One Command)

Open Terminal and run:

```bash
git clone https://github.com/miguelemosreverte/the-family-vpn.git && cd the-family-vpn && ./scripts/install.sh
```

That's it! The script will:
1. Install Go (if needed)
2. Build the VPN binaries
3. Configure auto-start on boot
4. Start the VPN connection
5. Show you the status

## What You'll See

```
════════════════════════════════════════════════════════════
  Family VPN Client Installer
════════════════════════════════════════════════════════════

▶ Detected: macos (arm64)
⚠ This script requires sudo access to create the VPN tunnel.
⚠ You will be prompted for your password.

✔ Go is already installed: go1.22.0
▶ Updating existing repository...
✔ Repository ready at /Users/you/the-family-vpn
▶ Building VPN binaries...
▶ Signing binaries for macOS...
✔ Binaries built successfully
▶ Installing launchd service for auto-start...
✔ launchd service installed
▶ Starting VPN client...
▶ Verifying VPN connection...
✔ VPN process is running
✔ VPN is connected!

════════════════════════════════════════════════════════════
  Installation Complete!
════════════════════════════════════════════════════════════

Your VPN client is now installed and running.

Node name: macbook-air-anastasiia
Server: 95.217.238.72:443
```

## After Installation

### Check if VPN is Working

```bash
~/the-family-vpn/bin/vpn status
```

You should see:
```
VPN Node Status
─────────────────────────────
Mode:       Client
VPN IP:     10.8.0.X
Server:     95.217.238.72:443
Connected:  Yes
```

### View All Connected Devices

```bash
~/the-family-vpn/bin/vpn peers
```

Shows all family devices connected to the VPN.

### Open the Dashboard

```bash
~/the-family-vpn/bin/vpn ui
```

Then open http://localhost:8080 in your browser.

## Troubleshooting

### VPN Not Starting

Check the logs:
```bash
sudo cat /var/log/vpn-node.log
```

### Restart the VPN

**macOS:**
```bash
sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist
sudo launchctl load /Library/LaunchDaemons/com.family.vpn-node.plist
```

**Linux:**
```bash
sudo systemctl restart vpn-node
```

### Password Prompt

The VPN needs administrator access to create the network tunnel. You'll be asked for your computer password during installation.

### Can't Reach VPN Server

Make sure you have internet access:
```bash
ping 95.217.238.72
```

If this fails, check your internet connection.

## Uninstall

To remove the VPN:

**macOS:**
```bash
sudo launchctl unload /Library/LaunchDaemons/com.family.vpn-node.plist
sudo rm /Library/LaunchDaemons/com.family.vpn-node.plist
rm -rf ~/the-family-vpn
```

**Linux:**
```bash
sudo systemctl stop vpn-node
sudo systemctl disable vpn-node
sudo rm /etc/systemd/system/vpn-node.service
rm -rf ~/the-family-vpn
```

## How It Works

```
Your Computer                          Server (Helsinki)
┌──────────────┐                      ┌──────────────┐
│              │   Encrypted Tunnel   │              │
│  VPN Client  │◄────────────────────►│  VPN Server  │
│  10.8.0.X    │     Port 443         │  10.8.0.1    │
│              │                      │              │
└──────────────┘                      └──────────────┘
       │                                     │
       │                                     │
       ▼                                     ▼
   Your Apps                           Other Family
  can access                            Devices
  10.8.0.0/24                          on the VPN
```

- All traffic between family devices goes through the encrypted tunnel
- Each device gets a VPN IP (10.8.0.X)
- The server in Helsinki connects everyone together
- Auto-reconnects if connection drops
- Starts automatically on boot
