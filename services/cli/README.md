# CLI Service

**Layer**: HOT (no downtime updates)

The CLI (`vpn` binary) is used to interact with the VPN node daemon. Updates to the CLI do NOT require restarting the VPN connection.

## What's Included

- `cmd/vpn/main.go` - CLI entry point
- `internal/cli/` - CLI client library

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
