# CLI Service

**Layer**: HOT (no downtime updates)

The CLI (`vpn` binary) is used to interact with the VPN node daemon. Updates to the CLI do NOT require restarting the VPN connection.

## What's Included

- `cmd/vpn/main.go` - CLI entry point
- `internal/cli/` - CLI client library

## Available Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `status` | - | Show node status (name, version, uptime, VPN IP) |
| `diagnose` | `diag`, `doctor`, `health` | Run comprehensive VPN connectivity diagnostics |
| `peers` | - | List connected peers |
| `network-peers` | `np`, `net-peers` | List all peers in the VPN network |
| `verify` | - | Verify VPN routing is working |
| `connect` | - | Enable VPN routing (route all traffic through VPN) |
| `disconnect` | - | Disable VPN routing (restore direct traffic) |
| `ssh` | - | SSH to a peer via VPN |
| `update` | - | Update node(s) with --all and --rolling options |
| `logs` | - | Query logs with Splunk-like time syntax |
| `stats` | - | Query metrics with Splunk-like time syntax |
| `crashes` | `crash`, `crash-stats` | Show crash statistics and last crash details |
| `lifecycle` | `events`, `history` | Show recent lifecycle events |
| `handshake` | - | Send install handshake to server |
| `handshakes` | - | Show install handshake history |
| `ui` | - | Start web dashboard |
| `version` | - | Show CLI and node version |

## Diagnostics

The `diagnose` command runs comprehensive connectivity checks and displays results in two sections: **This Node** (local checks) and **Network Peers** (per-peer diagnostics).

```bash
vpn diagnose           # Run all diagnostics
vpn diagnose -v        # Verbose output with details
vpn diagnose --json    # JSON output for scripting
```

### This Node Checks

1. **Local VPN Node** - Is the daemon running and responding?
2. **VPN Server** - Can we ping 10.8.0.1?
3. **Traffic Routing** - Is traffic routed through VPN?
4. **DNS Resolution** - Is DNS working?
5. **VPN Interface** - Is the TUN interface up?
6. **Internet Connectivity** - Can we reach external hosts?
7. **SSH Access** - Is Remote Login (SSH) enabled?

### Network Peers Checks

For each discovered peer:
- **Reachability** - Can we ping the peer's VPN IP?
- **Version** - What version is the peer running? (warns on mismatch)
- **Routing** - Is the peer routing through VPN? (warns if direct)
- **SSH Access** - Is SSH port 22 accessible?

Example output:
```
VPN Connectivity Diagnostics
═══════════════════════════════════════════════════════════════
Timestamp: 2025-12-07T20:28:11Z

This Node
───────────────────────────────────────────────────────────────
  Name:    miguels-macbook-air-local
  Version: fed3efc
  VPN IP:  10.8.0.2

[PASS] Local VPN Node       miguels-macbook-air-local (vfed3efc) - VPN IP: 10.8.0.2
[PASS] VPN Server (10.8.0.1) Server reachable
[PASS] Traffic Routing      Traffic routed through VPN
[PASS] DNS Resolution       DNS working
[PASS] VPN Interface        Interface utun0 is UP
[PASS] Internet Connectivity Internet reachable
[PASS] SSH Access           SSH enabled

Network Peers
───────────────────────────────────────────────────────────────
[PASS] Helsinki (10.8.0.1)
         OS: linux
         Routing: VPN (IP: 95.217.238.72)
         SSH: Accessible

[PASS] miguel-lemoss-Mac-mini.local (10.8.0.3)
         Version: fed3efc
         OS: darwin
         Routing: VPN (IP: 95.217.238.72)
         SSH: Accessible

───────────────────────────────────────────────────────────────
Summary: 9 passed, 0 failed, 0 warnings
```

### Warnings

The diagnose command shows warnings for:
- **Version mismatch** - Peer running a different version than local node
- **Not routing through VPN** - Peer's traffic going direct instead of through VPN
- **SSH not accessible** - Cannot reach SSH port 22 on peer

## Deployment

When `VERSION` changes, only the CLI binary is rebuilt. The VPN node continues running uninterrupted.

```bash
./deploy.sh
```

## When to Bump VERSION

Bump the CLI VERSION when:
- Adding new CLI commands
- Fixing CLI bugs
- Changing CLI output format
- Updating CLI dependencies (that don't affect node)

Do NOT bump CLI VERSION for:
- Node daemon changes (bump `core/VERSION` instead)
- Changes that require node restart
