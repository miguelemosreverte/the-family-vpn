#!/bin/bash
#
# VPN System Health Validation Script
# Outputs a markdown report with checkboxes for each feature validation
# Persists the report to INSTALL_DIR/HEALTH_REPORT.md
#
# Usage: ./scripts/validate.sh
#

INSTALL_DIR="$HOME/the-family-vpn"
VPN_BIN="$INSTALL_DIR/bin/vpn"
REPORT_FILE="$INSTALL_DIR/HEALTH_REPORT.md"

# Load .env if it exists (for SUDO_PASSWORD)
if [[ -f "$INSTALL_DIR/.env" ]]; then
    source "$INSTALL_DIR/.env"
fi

# Helper function to run sudo with password from .env
run_sudo() {
    if [[ -n "$SUDO_PASSWORD" ]]; then
        echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null
    else
        sudo "$@" 2>/dev/null
    fi
}

# Temporary file for building report
TEMP_REPORT=$(mktemp)

# Helper function to write to report
report() {
    echo "$1" >> "$TEMP_REPORT"
}

# Start report
report "# VPN System Health Report"
report ""
report "_Generated: $(date '+%Y-%m-%d %H:%M:%S')_"
report ""

# ============================================================
# 1. Installation Status
# ============================================================
report "## 1. Installation"
report ""

# Check VPN binary
if [[ -x "$VPN_BIN" ]]; then
    report "- [x] VPN binary installed at \`$VPN_BIN\`"
else
    report "- [ ] VPN binary installed at \`$VPN_BIN\`"
fi

# Check templates directory
if [[ -d "$INSTALL_DIR/internal/ui/templates" ]]; then
    report "- [x] UI templates directory exists"
else
    report "- [ ] UI templates directory exists"
fi

# Check git repo
if [[ -d "$INSTALL_DIR/.git" ]]; then
    cd "$INSTALL_DIR"
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null)
    GIT_DATE=$(git log -1 --format="%ci" 2>/dev/null | cut -d' ' -f1)
    report "- [x] Git repository (\`$GIT_COMMIT\` from $GIT_DATE)"
else
    report "- [ ] Git repository"
fi

report ""

# ============================================================
# 2. Services Status
# ============================================================
report "## 2. Services"
report ""

# Check VPN node process
if pgrep -f "vpn-node" > /dev/null; then
    VPN_PID=$(pgrep -f "vpn-node" | head -1)
    report "- [x] VPN Node process running (PID: $VPN_PID)"
else
    report "- [ ] VPN Node process running"
fi

# Check UI process
if pgrep -f "vpn.*ui" > /dev/null || curl -s -o /dev/null -w "%{http_code}" --max-time 2 http://localhost:8080/ 2>/dev/null | grep -q "200"; then
    report "- [x] UI service running"
else
    report "- [ ] UI service running"
fi

# Check launchd services (macOS)
if [[ "$OSTYPE" == "darwin"* ]]; then
    report ""
    report "### macOS LaunchDaemons"
    report ""
    for svc in vpn-node vpn-ui; do
        # Use launchctl print to check if service is loaded
        if run_sudo launchctl print system/com.family.$svc &>/dev/null; then
            report "- [x] \`com.family.$svc\` loaded"
        else
            report "- [ ] \`com.family.$svc\` loaded"
        fi
    done
fi

report ""

# ============================================================
# 3. VPN Connection
# ============================================================
report "## 3. VPN Connection"
report ""

if [[ -x "$VPN_BIN" ]]; then
    STATUS_OUTPUT=$("$VPN_BIN" status 2>&1)
    # Check if we have a VPN IP assigned (indicates connected)
    VPN_IP=$(echo "$STATUS_OUTPUT" | grep -i "VPN IP:" | sed 's/.*: *//' | tr -d '[:space:]')

    if [[ -n "$VPN_IP" && "$VPN_IP" =~ ^10\.8\. ]]; then
        report "- [x] Connected to VPN server"
        NODE_NAME=$(echo "$STATUS_OUTPUT" | grep -i "Name:" | sed 's/.*: *//')
        UPTIME=$(echo "$STATUS_OUTPUT" | grep -i "Uptime:" | sed 's/.*: *//')
        report "  - Node: \`$NODE_NAME\`"
        report "  - VPN IP: \`$VPN_IP\`"
        [[ -n "$UPTIME" ]] && report "  - Uptime: $UPTIME"
    else
        report "- [ ] Connected to VPN server"
    fi
else
    report "- [ ] VPN binary available"
fi

# Check VPN server reachability
if ping -c 1 -W 2 10.8.0.1 &>/dev/null; then
    LATENCY=$(ping -c 1 10.8.0.1 2>/dev/null | grep "time=" | sed 's/.*time=//' | sed 's/ .*//')
    report "- [x] VPN server reachable (10.8.0.1, ${LATENCY})"
else
    report "- [ ] VPN server reachable (10.8.0.1)"
fi

report ""

# ============================================================
# 4. Network Routing
# ============================================================
report "## 4. Network Routing"
report ""

# Check public IP via VPN
PUBLIC_IP=$(curl -s --max-time 5 https://api.ipify.org 2>/dev/null)
EXPECTED_IP="95.217.238.72"

if [[ -n "$PUBLIC_IP" ]]; then
    if [[ "$PUBLIC_IP" == "$EXPECTED_IP" ]]; then
        report "- [x] Traffic routed through VPN (\`$PUBLIC_IP\`)"
    else
        report "- [ ] Traffic routed through VPN (current: \`$PUBLIC_IP\`, expected: \`$EXPECTED_IP\`)"
    fi
else
    report "- [ ] Public IP detection"
fi

# DNS test
if host google.com &>/dev/null; then
    report "- [x] DNS resolution working"
else
    report "- [ ] DNS resolution working"
fi

report ""

# ============================================================
# 5. Peers
# ============================================================
report "## 5. Network Peers"
report ""

if [[ -x "$VPN_BIN" ]]; then
    PEERS_OUTPUT=$("$VPN_BIN" peers 2>&1)
    PEER_COUNT=$(echo "$PEERS_OUTPUT" | grep -c "10\.8\." 2>/dev/null || echo "0")
    PEER_COUNT=$(echo "$PEER_COUNT" | tr -d '[:space:]')

    if [[ "$PEER_COUNT" =~ ^[0-9]+$ ]] && [[ "$PEER_COUNT" -gt 0 ]]; then
        report "- [x] Peers discovered ($PEER_COUNT peers)"
        report ""
        report "| Name | VPN IP | Status |"
        report "|------|--------|--------|"
        echo "$PEERS_OUTPUT" | grep "10\.8\." | while read -r line; do
            NAME=$(echo "$line" | awk '{print $1}')
            IP=$(echo "$line" | grep -oE '10\.8\.[0-9]+\.[0-9]+' | head -1)
            report "| $NAME | \`$IP\` | Online |"
        done
    else
        report "- [ ] Peers discovered"
    fi
else
    report "- [ ] Peer discovery (VPN binary not found)"
fi

report ""

# ============================================================
# 6. Handshakes
# ============================================================
report "## 6. Install Handshakes"
report ""

if [[ -x "$VPN_BIN" ]]; then
    HANDSHAKES_OUTPUT=$("$VPN_BIN" handshakes 2>&1)
    HANDSHAKE_COUNT=$(echo "$HANDSHAKES_OUTPUT" | grep -c "20[0-9][0-9]-" || echo "0")

    if [[ "$HANDSHAKE_COUNT" -gt 0 ]]; then
        report "- [x] Handshakes recorded ($HANDSHAKE_COUNT entries)"
        report ""
        report "| Timestamp | Node | VPN IP |"
        report "|-----------|------|--------|"
        echo "$HANDSHAKES_OUTPUT" | grep "20[0-9][0-9]-" | head -5 | while read -r line; do
            TS=$(echo "$line" | grep -oE '20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9:]+' | head -1)
            NODE=$(echo "$line" | awk '{for(i=1;i<=NF;i++) if($i ~ /^[a-z]/ && $i !~ /darwin|linux|arm|amd/) {print $i; exit}}')
            IP=$(echo "$line" | grep -oE '10\.8\.[0-9]+\.[0-9]+' | head -1)
            report "| $TS | $NODE | \`$IP\` |"
        done
        [[ "$HANDSHAKE_COUNT" -gt 5 ]] && report "| ... | _${HANDSHAKE_COUNT} total_ | |"
    else
        report "- [ ] Handshakes recorded"
    fi
else
    report "- [ ] Handshakes (VPN binary not found)"
fi

report ""

# ============================================================
# 7. UI Dashboard
# ============================================================
report "## 7. UI Dashboard"
report ""

# Check if UI is accessible
UI_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://localhost:8080/ 2>/dev/null)
if [[ "$UI_RESPONSE" == "200" ]]; then
    report "- [x] Dashboard accessible (http://localhost:8080)"
else
    report "- [ ] Dashboard accessible (HTTP $UI_RESPONSE)"
fi

# Check API endpoints
API_STATUS=$(curl -s --max-time 3 http://localhost:8080/api/status 2>/dev/null)
if [[ -n "$API_STATUS" ]] && echo "$API_STATUS" | grep -q "node_name"; then
    report "- [x] API \`/api/status\` working"
else
    report "- [ ] API \`/api/status\` working"
fi

API_PEERS=$(curl -s --max-time 3 http://localhost:8080/api/peers 2>/dev/null)
if [[ -n "$API_PEERS" ]]; then
    report "- [x] API \`/api/peers\` working"
else
    report "- [ ] API \`/api/peers\` working"
fi

API_HANDSHAKES=$(curl -s --max-time 3 http://localhost:8080/api/handshakes 2>/dev/null)
if [[ -n "$API_HANDSHAKES" ]] && echo "$API_HANDSHAKES" | grep -q "entries"; then
    HS_COUNT=$(echo "$API_HANDSHAKES" | grep -o '"id"' | wc -l | tr -d ' ')
    report "- [x] API \`/api/handshakes\` working ($HS_COUNT entries)"
else
    report "- [ ] API \`/api/handshakes\` working"
fi

report ""

# ============================================================
# Summary
# ============================================================
report "---"
report ""

# Count passed/failed checks
PASSED=$(grep -c "\[x\]" "$TEMP_REPORT")
FAILED=$(grep -c "\[ \]" "$TEMP_REPORT")
TOTAL=$((PASSED + FAILED))

report "**Summary**: $PASSED/$TOTAL checks passed"
report ""
report "_Report saved to \`HEALTH_REPORT.md\`_"

# Move temp report to final location
mv "$TEMP_REPORT" "$REPORT_FILE"

# Output to terminal as well
cat "$REPORT_FILE"
