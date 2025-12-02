#!/bin/bash
# Detect which services have changed and need redeployment
# Usage: ./detect-changes.sh [old-commit] [new-commit]
#
# Returns a list of service directories that need to be deployed.
# Detection is based on:
# 1. VERSION file changes
# 2. Files within the service directory changing

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SERVICES_DIR="$ROOT_DIR/services"

OLD_COMMIT="${1:-HEAD~1}"
NEW_COMMIT="${2:-HEAD}"

echo "=== Detecting changes between $OLD_COMMIT and $NEW_COMMIT ===" >&2

# Get list of changed files
CHANGED_FILES=$(git diff --name-only "$OLD_COMMIT" "$NEW_COMMIT" 2>/dev/null || echo "")

if [ -z "$CHANGED_FILES" ]; then
    echo "No changes detected" >&2
    exit 0
fi

echo "Changed files:" >&2
echo "$CHANGED_FILES" | sed 's/^/  /' >&2

# Find all services with VERSION files
SERVICES=$(find "$SERVICES_DIR" -name "VERSION" -type f 2>/dev/null | xargs -I{} dirname {} | sort -u)

SERVICES_TO_DEPLOY=""

for SERVICE_PATH in $SERVICES; do
    SERVICE_NAME=$(basename "$SERVICE_PATH")
    SERVICE_REL_PATH="${SERVICE_PATH#$ROOT_DIR/}"

    # Check if VERSION file changed
    if echo "$CHANGED_FILES" | grep -q "^$SERVICE_REL_PATH/VERSION$"; then
        echo "  [VERSION] $SERVICE_NAME needs deployment (VERSION changed)" >&2
        SERVICES_TO_DEPLOY="$SERVICES_TO_DEPLOY $SERVICE_NAME"
        continue
    fi

    # Check if any file in the service directory changed
    if echo "$CHANGED_FILES" | grep -q "^$SERVICE_REL_PATH/"; then
        echo "  [FILES] $SERVICE_NAME needs deployment (files changed)" >&2
        SERVICES_TO_DEPLOY="$SERVICES_TO_DEPLOY $SERVICE_NAME"
        continue
    fi
done

# Also check for core code changes that affect all services
CORE_PATTERNS="internal/tunnel/ internal/protocol/ internal/node/ internal/cli/ cmd/"
for PATTERN in $CORE_PATTERNS; do
    if echo "$CHANGED_FILES" | grep -q "^$PATTERN"; then
        echo "  [CORE] core needs deployment ($PATTERN changed)" >&2
        if ! echo "$SERVICES_TO_DEPLOY" | grep -q "core"; then
            SERVICES_TO_DEPLOY="$SERVICES_TO_DEPLOY core"
        fi
    fi
done

# Output the list of services to deploy (one per line)
echo "" >&2
echo "Services to deploy:" >&2
for SERVICE in $SERVICES_TO_DEPLOY; do
    echo "  - $SERVICE" >&2
    echo "$SERVICE"
done
