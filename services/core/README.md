# Core VPN Service

The core VPN service handles:
- TUN device management
- VPN tunnel connections (TCP/TLS on port 443)
- Packet encryption/decryption
- Routing between peers

## Layer Classification

**COLD** - This service requires a full restart when updated. It cannot be hot-reloaded.

## Files

- `VERSION` - Current version number
- `deploy.sh` - Deployment script

## Deployment

Changes to this service affect the fundamental VPN connectivity. Deployment will:
1. Build new binaries
2. Restart the vpn-node service
3. Clients will briefly disconnect and reconnect

## Dependencies

None - this is the foundation layer.
