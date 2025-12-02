# VPN Mesh Network - Project Documentation

This document provides context for Claude Code sessions working on this project.

## Project Overview

A peer-to-peer mesh VPN network where every node runs identical software. Unlike traditional client-server VPNs, any node can act as a server or client.

**Key Philosophy**: Same binary everywhere. A node on Hetzner and a node on your MacBook run the same code.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         THE MESH NETWORK                             │
│                                                                      │
│    ┌──────────┐         ┌──────────┐         ┌──────────┐          │
│    │  Node A  │◄───────►│  Node B  │◄───────►│  Node C  │          │
│    │ (Hetzner)│         │ (MacBook)│         │(Mac Mini)│          │
│    │10.8.0.1  │         │10.8.0.2  │         │10.8.0.3  │          │
│    └──────────┘         └──────────┘         └──────────┘          │
│         ▲                                                           │
│         └───────────────────────────────────────────────────────────┘
└─────────────────────────────────────────────────────────────────────┘
```

## Two Components

### 1. Node Daemon (`vpn-node`)

Long-running process that:
- Maintains VPN tunnel connections
- Creates and manages TUN device
- Routes packets between peers
- Exposes control socket for CLI

```bash
# Server mode (accepts connections)
sudo vpn-node --server --vpn-addr 10.8.0.1 --listen-vpn :8443

# Client mode (connects to server)
sudo vpn-node --connect 95.217.238.72:8443

# Client mode with full traffic routing
sudo vpn-node --connect 95.217.238.72:8443 --route-all
```

### 2. CLI Tool (`vpn`)

Short-lived commands to interact with nodes. Features Splunk-like time range syntax for logs and metrics.

```bash
# Basic commands
vpn status                    # Show node status
vpn peers                     # List connected peers
vpn update                    # Update this node

# Logs (Splunk-like queries)
vpn logs                      # Last 15 minutes
vpn logs --earliest=-1h       # Last hour
vpn logs --level=ERROR        # Only errors
vpn logs --search="TUN"       # Search in messages

# Metrics (Splunk-like queries)
vpn stats                     # Current metrics
vpn stats --earliest=-1h      # Last hour
vpn stats --format=json       # JSON output for UI

# Remote node queries
vpn --node 10.8.0.1:9001 status

# Verify VPN routing
vpn verify --expected=95.217.238.72

# Start web dashboard
vpn ui                        # http://localhost:8080

# Interactive demo
./demo.sh
```

See `.claude/skills/vpn-cli.md` for full CLI documentation.

## Directory Structure

```
vpn/
├── cmd/
│   ├── vpn-node/main.go      # Node daemon entry point
│   └── vpn/main.go           # CLI entry point
├── internal/
│   ├── node/
│   │   ├── daemon.go         # Main daemon: server/client modes, packet routing
│   │   └── control.go        # CLI request handlers (status, peers, logs, stats)
│   ├── cli/
│   │   └── client.go         # Connects to node control socket
│   ├── tunnel/
│   │   ├── tun.go            # TUN device: create, configure, routing
│   │   ├── crypto.go         # AES-256-GCM encryption
│   │   └── conn.go           # VPN connection: dial, listen, read/write packets
│   ├── protocol/
│   │   ├── control.go        # CLI<->Node JSON-RPC messages
│   │   └── vpn.go            # VPN wire protocol: handshake, control messages
│   └── store/
│       ├── store.go          # SQLite storage for logs and metrics
│       ├── query.go          # Log and metric query engine
│       ├── timerange.go      # Splunk-like time range parser
│       ├── collector.go      # Metrics collection and aggregation
│       └── logger.go         # Structured logging to SQLite
├── services/
│   ├── core/
│   │   ├── VERSION           # Service version (triggers deployment)
│   │   ├── deploy.sh         # How to deploy this service
│   │   └── README.md         # Service documentation
│   └── websocket/
│       ├── VERSION
│       ├── deploy.sh
│       └── README.md
├── scripts/
│   ├── detect-changes.sh     # Detect which services need deployment
│   └── deploy.sh             # Main deployment orchestrator
├── .claude/
│   └── skills/
│       └── vpn-cli.md        # CLI skill documentation
├── demo.sh                   # Interactive CLI demo
├── go.mod
├── Makefile
└── CLAUDE.md                 # This file
```

## Key Files to Understand

| File | Purpose |
|------|---------|
| `internal/node/daemon.go` | Core daemon logic: starts server/client, routes packets |
| `internal/tunnel/tun.go` | TUN device creation (darwin/linux), routing table management |
| `internal/tunnel/conn.go` | VPN connections: TCP/TLS, encrypted packet read/write |
| `internal/protocol/vpn.go` | Wire protocol: handshake format, control messages |

## Wire Protocol

### Handshake (client → server)
```
[1 byte: encryption flag]
[4 bytes: peer info length]
[N bytes: peer info JSON]
```

### Server Response
```
[4 bytes: assigned IP length]
[N bytes: assigned IP string]
```

### Packet Format
```
[4 bytes: length (big endian)]
[N bytes: encrypted payload]
```

Encryption: AES-256-GCM (nonce prepended to ciphertext, ~28 bytes overhead)

## Deployment Strategy

Services have VERSION files. When VERSION changes, that service needs deployment.

```bash
# Deploy only changed services
./scripts/deploy.sh --changed

# Deploy specific service
./scripts/deploy.sh core

# Deploy across network (git pull + restart on all nodes)
./scripts/deploy.sh --network
```

### Service Layers (Cold → Hot)

| Layer | Services | Update Strategy |
|-------|----------|-----------------|
| FROZEN | TUN, VPN tunnel | Full restart required |
| COLD | WebSocket, Control | Restart, connections lost briefly |
| WARM | Routing policies | Graceful reload |
| HOT | Plugins, features | Hot swap, no downtime |

## Development Commands

```bash
# Build
make build                # Build both binaries
make build-linux          # Cross-compile for server

# Run locally
make run-node             # Run node in dev mode

# Test
go run test_connection.go server  # Start test server
go run test_connection.go client  # Run test client

# Deploy
make deploy-server        # Deploy to Hetzner server
./scripts/deploy.sh --all # Deploy all services
```

## Server Details

- **IP**: 95.217.238.72 (Hetzner, Helsinki)
- **SSH**: `ssh root@95.217.238.72`
- **VPN Port**: 443 (looks like HTTPS)
- **WebSocket Port**: 9000
- **VPN Subnet**: 10.8.0.0/24
- **Server VPN IP**: 10.8.0.1

## Common Tasks

### Add a new CLI command
1. Add handler in `internal/node/control.go`
2. Add client method in `internal/cli/client.go`
3. Add command in `cmd/vpn/main.go`

### Add a new service
1. Create `services/<name>/VERSION` with initial version
2. Create `services/<name>/deploy.sh` with deployment logic
3. Create `services/<name>/README.md` documenting the service

### Test VPN without root
Use `test_connection.go` which tests TCP + encryption without TUN:
```bash
go run test_connection.go server  # Terminal 1
go run test_connection.go client  # Terminal 2
```

## Reference Implementation

There's a working (but bloated) VPN at `/Users/miguel_lemos/Desktop/family-vpn/`. Key files:
- `client/main.go` - Working client implementation
- `server/main.go` - Working server implementation

The family-vpn has connectivity issues and all-at-once deployment. This project aims to fix those.

## Current Status

- [x] TUN device creation (darwin/linux)
- [x] AES-256-GCM encryption
- [x] VPN tunnel connection (TCP)
- [x] Handshake protocol
- [x] CLI with status/peers/update
- [x] Microservice deployment structure
- [x] SQLite storage for logs and metrics (50MB limit, auto-eviction)
- [x] Splunk-like time range queries (-1h, @d, etc.)
- [x] CLI logs command with filtering (level, component, search)
- [x] CLI stats command with JSON output for UI
- [x] Metrics collection (bandwidth, packets, uptime, latency)
- [x] Interactive demo script (demo.sh)
- [x] Deployed to Hetzner (95.217.238.72:443)
- [x] Full traffic routing (--route-all flag)
- [x] VPN verification command (vpn verify)
- [x] NAT configured on Helsinki server
- [x] Web dashboard (vpn ui)
- [ ] Real-time log streaming (--follow)
- [ ] WebSocket peer discovery
- [ ] TLS support
- [ ] Multi-hop routing
- [ ] Rolling updates
