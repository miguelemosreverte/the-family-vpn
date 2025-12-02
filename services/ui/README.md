# UI Service

**Layer**: HOT (no downtime updates)

The web dashboard served by `vpn ui` command. Updates to the UI do NOT require restarting the VPN connection.

## What's Included

- `cmd/vpn/main.go` - Contains UI HTTP handlers
- Any future `web/` static assets

## Deployment

When `VERSION` changes, the CLI binary is rebuilt (which includes the UI). The VPN node continues running uninterrupted.

```bash
./deploy.sh
```

## When to Bump VERSION

Bump the UI VERSION when:
- Changing dashboard layout
- Adding new UI features
- Fixing UI bugs
- Updating HTML/CSS/JS

Do NOT bump UI VERSION for:
- CLI command changes (bump `cli/VERSION`)
- Node daemon changes (bump `core/VERSION`)
