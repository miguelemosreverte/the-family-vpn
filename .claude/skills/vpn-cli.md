# VPN CLI Skill

This skill provides guidance for using the VPN command-line interface to query logs, metrics, and manage VPN nodes.

## Overview

The `vpn` CLI connects to VPN node daemons and provides Splunk-like querying capabilities for logs and metrics stored in local SQLite databases.

## Quick Start

```bash
# Run interactive demo
./demo.sh

# Basic commands
vpn status                    # Node status
vpn peers                     # Connected peers
vpn logs                      # Recent logs
vpn stats                     # Current metrics
vpn update                    # Update node
vpn ui                        # Start web dashboard
```

## Commands

### `vpn status`
Show current node status including name, version, uptime, VPN IP, peer count, and traffic statistics.

```bash
vpn status
vpn --node 10.8.0.1:9001 status   # Query remote node
```

### `vpn peers`
List all connected VPN peers with their names, VPN IPs, public IPs, and connection time.

```bash
vpn peers
```

### `vpn logs`
Query logs with Splunk-like time range syntax.

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--earliest` | Start time (Splunk syntax) | `-15m` |
| `--latest` | End time (Splunk syntax) | `now` |
| `--level` | Filter by level(s): DEBUG, INFO, WARN, ERROR | all |
| `--component` | Filter by component(s): conn, tun, node, store | all |
| `--search` | Full-text search in message | none |
| `--limit` | Max entries to return | 100 |

**Examples:**
```bash
vpn logs                                    # Last 15 minutes
vpn logs --earliest=-1h                     # Last hour
vpn logs --earliest=-24h --latest=-1h       # 24h ago to 1h ago
vpn logs --level=ERROR                      # Only errors
vpn logs --level=WARN,ERROR                 # Warnings and errors
vpn logs --search="connection"              # Search in messages
vpn logs --component=conn                   # Only conn component
vpn logs --component=conn,tun               # Multiple components
vpn logs --limit=50                         # Limit results
vpn logs --earliest=@d                      # Since midnight today
vpn logs --earliest=-1h@h                   # Last hour, snapped to hour
```

### `vpn stats`
Query metrics with Splunk-like time range syntax.

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--earliest` | Start time (Splunk syntax) | `-5m` |
| `--latest` | End time (Splunk syntax) | `now` |
| `--metric` | Specific metric(s) to query | all |
| `--granularity` | Data resolution: raw, 1m, 1h, auto | `auto` |
| `--format` | Output format: text, json | `text` |

**Available Metrics:**
| Metric | Description |
|--------|-------------|
| `vpn.bytes_sent` | Total bytes transmitted |
| `vpn.bytes_recv` | Total bytes received |
| `vpn.packets_sent` | Total packets transmitted |
| `vpn.packets_recv` | Total packets received |
| `vpn.active_peers` | Number of connected peers |
| `vpn.uptime_seconds` | Node uptime in seconds |
| `vpn.latency_ms` | Current latency |
| `vpn.packet_loss_pct` | Packet loss percentage |
| `bandwidth.tx_current_bps` | Current TX bandwidth (bytes/sec) |
| `bandwidth.rx_current_bps` | Current RX bandwidth (bytes/sec) |
| `bandwidth.tx_avg_bps` | Average TX bandwidth |
| `bandwidth.rx_avg_bps` | Average RX bandwidth |
| `bandwidth.tx_peak_bps` | Peak TX bandwidth |
| `bandwidth.rx_peak_bps` | Peak RX bandwidth |

**Examples:**
```bash
vpn stats                                   # Last 5 minutes, all metrics
vpn stats --earliest=-1h                    # Last hour
vpn stats --earliest=-2h --latest=-1h       # 2h ago to 1h ago
vpn stats --metric=bandwidth.tx_current_bps # Single metric
vpn stats --metric=vpn.bytes_sent,vpn.bytes_recv  # Multiple metrics
vpn stats --granularity=raw                 # 1-second resolution
vpn stats --granularity=1m                  # 1-minute aggregates
vpn stats --format=json                     # JSON for UI consumption
```

### `vpn update`
Trigger node updates (git pull + restart).

**Flags:**
| Flag | Description |
|------|-------------|
| `--all` | Update all nodes in network |
| `--rolling` | Update one node at a time (requires --all) |

**Examples:**
```bash
vpn update                    # Update this node
vpn update --all              # Update all nodes
vpn update --all --rolling    # Rolling update
```

### `vpn verify`
Verify VPN routing is working correctly by checking public IP.

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--expected` | Expected public IP (VPN server IP) | none |

**Examples:**
```bash
vpn verify                              # Check current public IP
vpn verify --expected=95.217.238.72     # Verify routing to Helsinki
```

**Output when routing is working:**
```
VPN Routing Verification
────────────────────────────────────────
  Public IP:     95.217.238.72
  VPN IP:        10.8.0.2
  Node:          my-macbook (v0.1.0)
  Uptime:        5m

  Routing:       VERIFIED
                 Traffic is routed through 95.217.238.72
```

### `vpn ui`
Start a web dashboard for monitoring VPN nodes.

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--listen` | Address to listen on | `localhost:8080` |

**Examples:**
```bash
vpn ui                              # Start on http://localhost:8080
vpn ui --listen :3000               # Start on port 3000
vpn --node 10.8.0.1:9001 ui         # Connect to remote node
```

**Dashboard Pages:**
- **Home**: Welcome page
- **Overview**: Node status, connected peers, bandwidth charts
- **Observability**: Splunk-like log viewer, metrics charts with time ranges
- **Peers**: Detailed list of all connected peers

## Time Range Syntax (Splunk-compatible)

### Relative Time
| Syntax | Meaning |
|--------|---------|
| `-1h` | 1 hour ago |
| `-30m` | 30 minutes ago |
| `-7d` | 7 days ago |
| `-1w` | 1 week ago |
| `+1h` | 1 hour from now |

### Snap to Boundary
| Syntax | Meaning |
|--------|---------|
| `@s` | Beginning of current second |
| `@m` | Beginning of current minute |
| `@h` | Beginning of current hour |
| `@d` | Beginning of current day (midnight) |
| `@w` | Beginning of current week (Monday) |
| `@M` | Beginning of current month |
| `@y` | Beginning of current year |

### Combined
| Syntax | Meaning |
|--------|---------|
| `-1h@h` | 1 hour ago, snapped to hour boundary |
| `-1d@d` | 1 day ago, snapped to midnight |
| `@d` | Midnight today |

### Absolute Time
| Syntax | Meaning |
|--------|---------|
| `2024-01-15` | Midnight on date |
| `2024-01-15T14:30:00` | Specific time |
| `2024-01-15T14:30:00Z` | UTC time |
| `1704067200` | Unix timestamp (seconds) |

### Special Keywords
| Syntax | Meaning |
|--------|---------|
| `now` | Current time |
| `today` | Midnight today |
| `yesterday` | Midnight yesterday |

## JSON Output for UI Integration

For programmatic consumption (dashboards, charts), use `--format=json`:

```bash
# Get bandwidth data for charting
vpn stats --earliest=-1h --metric=bandwidth.tx_current_bps --format=json
```

**Output structure:**
```json
{
  "series": [
    {
      "name": "bandwidth.tx_current_bps",
      "points": [
        {"timestamp": "2024-01-15T14:30:00Z", "value": 1024.5, "granularity": "raw"},
        {"timestamp": "2024-01-15T14:30:01Z", "value": 2048.0, "granularity": "raw"}
      ]
    }
  ],
  "summary": {
    "bandwidth.tx_current_bps": 1536.25
  },
  "storage_info": {
    "db_size_mb": 1.5,
    "log_count": 100,
    "metrics_raw_count": 5000
  }
}
```

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--node` | Address of node to connect to | `127.0.0.1:9001` |
| `--help` | Show help for command | - |

## Storage

- **Location:** `~/.vpn-node/vpn.db` (SQLite)
- **Max size:** 50 MB (auto-eviction of old data)
- **Retention:**
  - Raw metrics: 1 hour
  - 1-minute aggregates: 24 hours
  - 1-hour aggregates: 30 days
  - Logs: 7 days (subject to size limit)

## Interactive Demo

Run the interactive demo to explore all commands:

```bash
./demo.sh
```

Navigate with arrow keys or j/k, press Enter to run, q to quit.
