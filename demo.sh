#!/bin/bash
# VPN CLI Interactive Demo
# Navigate with arrow keys, select with Enter, quit with 'q'

# Get script directory (works regardless of where script is called from)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VPN_CLI="$SCRIPT_DIR/bin/vpn"
NODE_ADDR="127.0.0.1:9002"

# =============================================================================
# DEMONSTRATIONS - Add new demos here (declarative list)
# Format: "Title|Description|Command"
# =============================================================================
DEMOS=(
  # --- STATUS ---
  "Status|Show current node status (name, uptime, traffic)|status"

  # --- PEERS ---
  "Peers|List all connected VPN peers|peers"

  # --- LOGS ---
  "Logs (recent)|Last 15 minutes of logs|logs"
  "Logs (1 hour)|Last hour of logs|logs --earliest=-1h"
  "Logs (time range)|Logs from 24h ago to 1h ago|logs --earliest=-24h --latest=-1h"
  "Logs (errors)|Only ERROR level logs|logs --level=ERROR"
  "Logs (warnings+)|WARN and ERROR logs|logs --level=WARN,ERROR"
  "Logs (search)|Search for 'TUN' in messages|logs --search=TUN"
  "Logs (component)|Logs from 'conn' component only|logs --component=conn"
  "Logs (multi-component)|Logs from conn and tun|logs --component=conn,tun"
  "Logs (limit)|Show only last 10 entries|logs --limit=10"
  "Logs (today)|Logs since midnight today|logs --earliest=@d"

  # --- STATS ---
  "Stats (summary)|Current metrics summary (last 5 min)|stats"
  "Stats (1 hour)|Metrics from last hour|stats --earliest=-1h"
  "Stats (time range)|Metrics from 2h ago to 1h ago|stats --earliest=-2h --latest=-1h"
  "Stats (bandwidth)|Only TX/RX bandwidth metrics|stats --metric=bandwidth.tx_current_bps,bandwidth.rx_current_bps"
  "Stats (traffic)|Only bytes sent/received|stats --metric=vpn.bytes_sent,vpn.bytes_recv"
  "Stats (1m granularity)|Force 1-minute aggregation|stats --earliest=-1h --granularity=1m"
  "Stats (raw granularity)|Force raw 1-second data|stats --earliest=-5m --granularity=raw"
  "Stats (JSON)|JSON output for UI (1 metric)|stats --earliest=-1m --metric=bandwidth.tx_current_bps --format=json"
  "Stats (JSON all)|Full JSON output for UI|stats --format=json"

  # --- UPDATE ---
  "Update (this node)|Trigger update on this node|update"
  "Update (all nodes)|Update all nodes in network|update --all"
  "Update (rolling)|Rolling update (one at a time)|update --all --rolling"

  # --- VERIFY ---
  "Verify (public IP)|Check current public IP|verify"
  "Verify (routing)|Verify traffic routes through Helsinki|verify --expected=95.217.238.72"

  # --- CONNECTION CONTROL ---
  "Connect|Enable VPN routing (route all traffic through VPN)|connect"
  "Disconnect|Disable VPN routing (restore direct traffic)|disconnect"
  "Connection Status|Show current VPN connection status|connection-status"

  # --- UI ---
  "Dashboard|Start web dashboard (press Ctrl+C to stop)|ui"
)

# =============================================================================
# UI Code - No need to modify below this line
# =============================================================================

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color
BOLD='\033[1m'
REVERSE='\033[7m'

# State
selected=0
total=${#DEMOS[@]}

# Clear screen and hide cursor
clear_screen() {
  printf "\033[2J\033[H"
}

hide_cursor() {
  printf "\033[?25l"
}

show_cursor() {
  printf "\033[?25h"
}

# Draw the menu
draw_menu() {
  clear_screen

  echo -e "${BOLD}${CYAN}╔════════════════════════════════════════════════════════════════╗${NC}"
  echo -e "${BOLD}${CYAN}║${NC}            ${BOLD}${WHITE}VPN CLI Interactive Demo${NC}                          ${BOLD}${CYAN}║${NC}"
  echo -e "${BOLD}${CYAN}╠════════════════════════════════════════════════════════════════╣${NC}"
  echo -e "${BOLD}${CYAN}║${NC}  ${GRAY}Use ↑/↓ or j/k to navigate, Enter to run, q to quit${NC}         ${BOLD}${CYAN}║${NC}"
  echo -e "${BOLD}${CYAN}╚════════════════════════════════════════════════════════════════╝${NC}"
  echo ""

  for i in "${!DEMOS[@]}"; do
    IFS='|' read -r title description command <<< "${DEMOS[$i]}"

    if [ $i -eq $selected ]; then
      echo -e "  ${REVERSE}${WHITE} → ${title} ${NC}"
      echo -e "     ${GRAY}${description}${NC}"
      echo -e "     ${CYAN}vpn ${command}${NC}"
    else
      echo -e "     ${WHITE}${title}${NC}"
    fi
    echo ""
  done

  echo -e "${GRAY}────────────────────────────────────────────────────────────────${NC}"
  echo -e "  ${GRAY}Node: ${NODE_ADDR}${NC}"
}

# Run selected demo
run_demo() {
  IFS='|' read -r title description command <<< "${DEMOS[$selected]}"

  clear_screen
  echo -e "${BOLD}${GREEN}Running: ${title}${NC}"
  echo -e "${GRAY}Command: ${VPN_CLI} --node ${NODE_ADDR} ${command}${NC}"
  echo -e "${GRAY}────────────────────────────────────────────────────────────────${NC}"
  echo ""

  # Run the command
  eval "${VPN_CLI} --node ${NODE_ADDR} ${command}"

  echo ""
  echo -e "${GRAY}────────────────────────────────────────────────────────────────${NC}"
  echo -e "${YELLOW}Press any key to return to menu...${NC}"
  read -rsn1
}

# Read single keypress
read_key() {
  local key
  IFS= read -rsn1 key

  # Check for escape sequence (arrow keys)
  if [[ $key == $'\x1b' ]]; then
    read -rsn2 -t 0.1 key
    case $key in
      '[A') echo "up" ;;
      '[B') echo "down" ;;
      *) echo "" ;;
    esac
  else
    case $key in
      'k') echo "up" ;;
      'j') echo "down" ;;
      'q') echo "quit" ;;
      '') echo "enter" ;;
      *) echo "" ;;
    esac
  fi
}

# Main loop
main() {
  # Check if binary exists
  if [ ! -f "$VPN_CLI" ]; then
    echo -e "${RED}Error: VPN CLI not found at ${VPN_CLI}${NC}"
    echo "Please build it first: go build -o bin/vpn ./cmd/vpn"
    exit 1
  fi

  hide_cursor
  trap 'show_cursor; exit' INT TERM

  while true; do
    draw_menu

    key=$(read_key)

    case $key in
      "up")
        ((selected--))
        if [ $selected -lt 0 ]; then
          selected=$((total - 1))
        fi
        ;;
      "down")
        ((selected++))
        if [ $selected -ge $total ]; then
          selected=0
        fi
        ;;
      "enter")
        show_cursor
        run_demo
        hide_cursor
        ;;
      "quit")
        show_cursor
        clear_screen
        echo -e "${GREEN}Goodbye!${NC}"
        exit 0
        ;;
    esac
  done
}

# Run
main
