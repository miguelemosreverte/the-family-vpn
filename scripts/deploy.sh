#!/bin/bash
# Main deployment script
# Usage: ./deploy.sh [service1] [service2] ...
#        ./deploy.sh --all          # Deploy all services
#        ./deploy.sh --changed      # Deploy only changed services
#        ./deploy.sh --network      # Deploy to all nodes in the network

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SERVICES_DIR="$ROOT_DIR/services"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse arguments
DEPLOY_ALL=false
DEPLOY_CHANGED=false
DEPLOY_NETWORK=false
SERVICES_TO_DEPLOY=()

while [[ $# -gt 0 ]]; do
    case $1 in
        --all)
            DEPLOY_ALL=true
            shift
            ;;
        --changed)
            DEPLOY_CHANGED=true
            shift
            ;;
        --network)
            DEPLOY_NETWORK=true
            shift
            ;;
        *)
            SERVICES_TO_DEPLOY+=("$1")
            shift
            ;;
    esac
done

# Determine which services to deploy
if [ "$DEPLOY_ALL" = true ]; then
    log_info "Deploying all services..."
    SERVICES_TO_DEPLOY=($(find "$SERVICES_DIR" -name "VERSION" -type f | xargs -I{} dirname {} | xargs -I{} basename {}))
elif [ "$DEPLOY_CHANGED" = true ]; then
    log_info "Detecting changed services..."
    SERVICES_TO_DEPLOY=($("$SCRIPT_DIR/detect-changes.sh"))
fi

if [ ${#SERVICES_TO_DEPLOY[@]} -eq 0 ]; then
    log_info "No services to deploy"
    exit 0
fi

log_info "Services to deploy: ${SERVICES_TO_DEPLOY[*]}"

# First, git pull to get latest code
log_info "Pulling latest code..."
cd "$ROOT_DIR"
git pull origin main || log_warn "Git pull failed (might be in detached HEAD state)"

# Deploy each service
FAILED_SERVICES=()
for SERVICE in "${SERVICES_TO_DEPLOY[@]}"; do
    SERVICE_DIR="$SERVICES_DIR/$SERVICE"
    DEPLOY_SCRIPT="$SERVICE_DIR/deploy.sh"

    if [ ! -f "$DEPLOY_SCRIPT" ]; then
        log_error "No deploy.sh found for service: $SERVICE"
        FAILED_SERVICES+=("$SERVICE")
        continue
    fi

    log_info "Deploying $SERVICE (version: $(cat $SERVICE_DIR/VERSION 2>/dev/null || echo 'unknown'))..."

    if bash "$DEPLOY_SCRIPT"; then
        log_info "$SERVICE deployed successfully"
    else
        log_error "$SERVICE deployment failed"
        FAILED_SERVICES+=("$SERVICE")
    fi
done

# Network-wide deployment
if [ "$DEPLOY_NETWORK" = true ]; then
    log_info "Triggering network-wide deployment..."

    # Use the CLI to send update command to all nodes
    if [ -x "$ROOT_DIR/bin/vpn" ]; then
        "$ROOT_DIR/bin/vpn" update --all --rolling
    else
        log_warn "vpn CLI not found, skipping network deployment"
    fi
fi

# Summary
echo ""
echo "=== Deployment Summary ==="
echo "Deployed: ${#SERVICES_TO_DEPLOY[@]} services"
if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
    log_error "Failed: ${FAILED_SERVICES[*]}"
    exit 1
else
    log_info "All deployments successful"
fi
