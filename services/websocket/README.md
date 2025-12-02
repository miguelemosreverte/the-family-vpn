# WebSocket Service

The WebSocket service handles:
- Peer discovery and registration
- Real-time event broadcasting
- Health ping monitoring
- Version synchronization

## Layer Classification

**COLD** - Currently embedded in the core service. Future versions may allow hot-reload.

## Port

9000 (HTTP/WebSocket)

## Protocol

JSON messages over WebSocket. See `internal/protocol/` for message formats.

## Endpoints

- `GET /ws?vpn_ip=<ip>` - WebSocket connection for real-time events
- `POST /update/init` - Trigger network-wide update

## Dependencies

- Core VPN service (for peer registry)
