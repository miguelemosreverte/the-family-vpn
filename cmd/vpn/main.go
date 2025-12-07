// vpn is the command-line interface for interacting with VPN nodes.
//
// Usage:
//
//	vpn [global flags] <command> [command flags]
//
// Commands:
//
//	status     Show node status
//	peers      List connected peers
//	diagnose   Run comprehensive VPN connectivity diagnostics
//	update     Update node(s)
//	logs       Query logs (Splunk-like)
//	stats      Query metrics (Splunk-like)
//	verify     Verify VPN routing is working
//	connect    Enable VPN routing (route all traffic through VPN)
//	disconnect Disable VPN routing (restore direct traffic)
//	ssh        SSH to a peer via VPN
//	handshake  Send install handshake to server
//	handshakes Show install handshake history
//
// Global Flags:
//
//	--node   Address of node to connect to (default: 127.0.0.1:9001)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/miguelemosreverte/vpn/internal/cli"
	"github.com/miguelemosreverte/vpn/internal/protocol"
	"github.com/miguelemosreverte/vpn/internal/ui"
)

var nodeAddr string

func main() {
	rootCmd := &cobra.Command{
		Use:   "vpn",
		Short: "VPN mesh network CLI",
		Long: `vpn is a command-line interface for interacting with VPN nodes.

By default, it connects to the local node at 127.0.0.1:9001.
Use --node to connect to a remote node.`,
	}

	rootCmd.PersistentFlags().StringVar(&nodeAddr, "node", "127.0.0.1:9001",
		"Address of node to connect to")

	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(peersCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(logsCmd())
	rootCmd.AddCommand(statsCmd())
	rootCmd.AddCommand(verifyCmd())
	rootCmd.AddCommand(uiCmd())
	rootCmd.AddCommand(connectCmd())
	rootCmd.AddCommand(disconnectCmd())
	rootCmd.AddCommand(connectionStatusCmd())
	rootCmd.AddCommand(sshCmd())
	rootCmd.AddCommand(networkPeersCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(crashesCmd())
	rootCmd.AddCommand(lifecycleCmd())
	rootCmd.AddCommand(handshakeCmd())
	rootCmd.AddCommand(handshakesCmd())
	rootCmd.AddCommand(diagnoseCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show node status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			status, err := client.Status()
			if err != nil {
				return err
			}

			fmt.Printf(`
Node Status
───────────────────────────────
  Name:       %s
  Version:    %s
  Uptime:     %s
  VPN IP:     %s
  Peers:      %d
  Traffic In: %s
  Traffic Out:%s
`, status.NodeName, status.Version, status.UptimeStr,
				status.VPNAddress, status.PeerCount,
				formatBytes(status.BytesIn), formatBytes(status.BytesOut))

			return nil
		},
	}
}

func peersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "peers",
		Short: "List connected peers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.Peers()
			if err != nil {
				return err
			}

			if len(result.Peers) == 0 {
				fmt.Println("No peers connected.")
				return nil
			}

			fmt.Println("\nConnected Peers")
			fmt.Println("───────────────────────────────────────────────────────")
			fmt.Printf("%-15s %-15s %-18s %s\n", "NAME", "VPN IP", "PUBLIC IP", "CONNECTED")
			fmt.Println("───────────────────────────────────────────────────────")

			for _, p := range result.Peers {
				fmt.Printf("%-15s %-15s %-18s %s\n",
					p.Name, p.VPNAddress, p.PublicIP,
					p.Connected.Format("2006-01-02 15:04"))
			}

			return nil
		},
	}
}

func updateCmd() *cobra.Command {
	var all, rolling bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update node(s)",
		Long: `Update triggers a git pull and restart on the node.

Use --all to update all nodes in the network.
Use --rolling with --all to update nodes one at a time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.Update(all, rolling)
			if err != nil {
				return err
			}

			if result.Success {
				fmt.Println("Update successful!")
				fmt.Printf("Updated nodes: %v\n", result.Updated)
			} else {
				fmt.Println("Update failed:")
				for _, e := range result.Errors {
					fmt.Printf("  - %s\n", e)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Update all nodes in the network")
	cmd.Flags().BoolVar(&rolling, "rolling", false, "Update nodes one at a time (requires --all)")

	return cmd
}

func logsCmd() *cobra.Command {
	var earliest, latest, search string
	var levels, components []string
	var limit int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Query logs (Splunk-like time syntax)",
		Long: `Query logs with Splunk-like time range syntax.

Time range examples:
  -1h        1 hour ago
  -30m       30 minutes ago
  -7d        7 days ago
  -1h@h      1 hour ago, snapped to hour boundary
  @d         Beginning of today
  now        Current time
  2024-01-15 Specific date

Usage examples:
  vpn logs                           # Last 15 minutes
  vpn logs --earliest=-1h            # Last hour
  vpn logs --earliest=-24h --latest=-1h  # 24h to 1h ago
  vpn logs --level=ERROR             # Only errors
  vpn logs --search="connection"     # Search in message
  vpn logs --component=conn,tun      # Filter by component`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			params := protocol.LogsParams{
				Earliest:   earliest,
				Latest:     latest,
				Levels:     levels,
				Components: components,
				Search:     search,
				Limit:      limit,
			}

			result, err := client.Logs(params)
			if err != nil {
				return err
			}

			if len(result.Entries) == 0 {
				fmt.Println("No logs found for the specified time range.")
				return nil
			}

			fmt.Printf("\nLogs (%d of %d)\n", len(result.Entries), result.TotalCount)
			fmt.Println("────────────────────────────────────────────────────────────────────")

			for _, e := range result.Entries {
				levelColor := getLevelColor(e.Level)
				fmt.Printf("%s %s[%-5s]%s [%s] %s\n",
					e.Timestamp[:19], levelColor, e.Level, colorReset,
					e.Component, e.Message)
			}

			if result.HasMore {
				fmt.Printf("\n... %d more entries (use --limit to see more)\n", result.TotalCount-int64(len(result.Entries)))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&earliest, "earliest", "-15m", "Start time (Splunk syntax: -1h, -30m, @d)")
	cmd.Flags().StringVar(&latest, "latest", "now", "End time (Splunk syntax)")
	cmd.Flags().StringSliceVar(&levels, "level", nil, "Filter by level (DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().StringSliceVar(&components, "component", nil, "Filter by component (conn, tun, node)")
	cmd.Flags().StringVar(&search, "search", "", "Search text in message")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max entries to return")

	return cmd
}

func statsCmd() *cobra.Command {
	var earliest, latest, granularity, format string
	var metrics []string

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Query metrics (Splunk-like time syntax)",
		Long: `Query metrics with Splunk-like time range syntax.

Available metrics:
  vpn.bytes_sent, vpn.bytes_recv       Traffic counters
  vpn.packets_sent, vpn.packets_recv   Packet counters
  vpn.active_peers                     Connected peers
  vpn.uptime_seconds                   Node uptime
  bandwidth.tx_current_bps             Current TX bandwidth
  bandwidth.rx_current_bps             Current RX bandwidth

Granularity:
  raw   High resolution (1 second)
  1m    1-minute aggregates
  1h    1-hour aggregates
  auto  Auto-select based on time range

Output formats:
  text  Human-readable output (default)
  json  JSON output with all data points (for UI/programmatic use)

Usage examples:
  vpn stats                            # Last 5 minutes, all metrics
  vpn stats --earliest=-1h             # Last hour
  vpn stats --metric=bandwidth.tx_current_bps,bandwidth.rx_current_bps
  vpn stats --granularity=1m           # Force 1-minute aggregation
  vpn stats --format=json              # JSON output for UI consumption`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			params := protocol.StatsParams{
				Earliest:    earliest,
				Latest:      latest,
				Metrics:     metrics,
				Granularity: granularity,
			}

			result, err := client.Stats(params)
			if err != nil {
				return err
			}

			// JSON output for programmatic use
			if format == "json" {
				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(output))
				return nil
			}

			// Print summary (latest values)
			fmt.Println("\nCurrent Metrics")
			fmt.Println("────────────────────────────────────────")

			for name, value := range result.Summary {
				displayName := strings.TrimPrefix(name, "vpn.")
				displayName = strings.TrimPrefix(displayName, "bandwidth.")

				// Format value based on metric type
				var formatted string
				if strings.Contains(name, "bytes") {
					formatted = formatBytes(uint64(value))
				} else if strings.Contains(name, "bps") {
					formatted = formatBandwidth(value)
				} else if strings.Contains(name, "uptime") {
					formatted = formatUptime(value)
				} else {
					formatted = fmt.Sprintf("%.0f", value)
				}

				fmt.Printf("  %-20s %s\n", displayName+":", formatted)
			}

			// Print storage info
			if len(result.StorageInfo) > 0 {
				fmt.Println("\nStorage")
				fmt.Println("────────────────────────────────────────")
				if dbSize, ok := result.StorageInfo["db_size_mb"]; ok {
					fmt.Printf("  %-20s %.2f MB\n", "database:", dbSize)
				}
				if logCount, ok := result.StorageInfo["log_count"]; ok {
					fmt.Printf("  %-20s %.0f entries\n", "logs:", logCount)
				}
				if rawCount, ok := result.StorageInfo["metrics_raw_count"]; ok {
					fmt.Printf("  %-20s %.0f points\n", "metrics (raw):", rawCount)
				}
			}

			// Print time series if available
			if len(result.Series) > 0 {
				fmt.Printf("\nTime Series (%d series)\n", len(result.Series))
				fmt.Println("────────────────────────────────────────")
				for _, s := range result.Series {
					if len(s.Points) > 0 {
						first := s.Points[0]
						last := s.Points[len(s.Points)-1]
						fmt.Printf("  %s: %d points (%s to %s)\n",
							s.Name, len(s.Points),
							first.Timestamp[:19], last.Timestamp[:19])
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&earliest, "earliest", "-5m", "Start time (Splunk syntax: -1h, -30m, @d)")
	cmd.Flags().StringVar(&latest, "latest", "now", "End time (Splunk syntax)")
	cmd.Flags().StringSliceVar(&metrics, "metric", nil, "Specific metrics to query")
	cmd.Flags().StringVar(&granularity, "granularity", "auto", "Data granularity (raw, 1m, 1h, auto)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

func uiCmd() *cobra.Command {
	var listenAddr string
	var templatesDir string

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start web dashboard",
		Long: `Start a web dashboard for monitoring VPN nodes.

The dashboard provides:
  - Home: Welcome page
  - Overview: Node status, peers, bandwidth charts
  - Observability: Splunk-like log viewer and metrics charts

Node selection priority:
  1. If --node is explicitly set, use that node
  2. Try local node at 127.0.0.1:9001 first (preferred for client perspective)
  3. Fall back to VPN server at 95.217.238.72:9001 if local isn't available

Examples:
  vpn ui                           # Start on http://localhost:8080
  vpn ui --listen :3000            # Start on port 3000
  vpn --node 10.8.0.1:9001 ui      # Connect to remote node
  vpn ui --templates ./internal/ui/templates  # Hot reload from disk`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine which node to connect to
			targetNode := nodeAddr

			// Only do smart detection if --node is still the default value
			// (the flag is on the root command, so we check value equality)
			if nodeAddr == "127.0.0.1:9001" {
				// Try local node first (127.0.0.1:9001)
				localAddr := "127.0.0.1:9001"
				client, err := cli.NewClient(localAddr)
				if err == nil {
					// Local node is available - use it for client perspective
					client.Close()
					targetNode = localAddr
					fmt.Printf("  Using local node at %s (client perspective)\n", localAddr)
				} else {
					// Local not available, try the server
					serverAddr := "95.217.238.72:9001"
					client, err = cli.NewClient(serverAddr)
					if err == nil {
						client.Close()
						targetNode = serverAddr
						fmt.Printf("  No local node found, using server at %s\n", serverAddr)
					} else {
						// Neither available - use default and let it fail with proper error
						fmt.Printf("  Warning: No VPN node found locally or on server\n")
					}
				}
			}

			server := ui.NewServer(targetNode, listenAddr)
			if templatesDir != "" {
				server.SetTemplatesDir(templatesDir)
				fmt.Printf("  Hot reload enabled: %s\n", templatesDir)
			}
			return server.Start()
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "localhost:8080", "Address to listen on")
	cmd.Flags().StringVar(&templatesDir, "templates", "", "Load templates from disk for hot reload (dev mode)")

	return cmd
}

func verifyCmd() *cobra.Command {
	var expectedIP string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify VPN routing is working",
		Long: `Verify that traffic is being routed through the VPN.

This command checks your public IP address and compares it to the expected
VPN server IP to confirm traffic is being routed correctly.

Examples:
  vpn verify                                # Check current public IP
  vpn verify --expected=95.217.238.72       # Verify routing to specific IP`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("\nVPN Routing Verification")
			fmt.Println("────────────────────────────────────────")

			// Get current public IP
			publicIP, err := getPublicIP()
			if err != nil {
				fmt.Printf("  Public IP:     %s (error: %v)\n", colorRed+"FAILED"+colorReset, err)
				return nil
			}

			fmt.Printf("  Public IP:     %s\n", publicIP)

			// Check node status for VPN IP
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				fmt.Printf("  Node Status:   %s (cannot connect to %s)\n", colorYellow+"UNKNOWN"+colorReset, nodeAddr)
			} else {
				defer client.Close()
				status, err := client.Status()
				if err != nil {
					fmt.Printf("  Node Status:   %s (error: %v)\n", colorYellow+"UNKNOWN"+colorReset, err)
				} else {
					fmt.Printf("  VPN IP:        %s\n", status.VPNAddress)
					fmt.Printf("  Node:          %s (v%s)\n", status.NodeName, status.Version)
					fmt.Printf("  Uptime:        %s\n", status.UptimeStr)
				}
			}

			// Verify against expected IP
			if expectedIP != "" {
				fmt.Println()
				if publicIP == expectedIP {
					fmt.Printf("  Routing:       %s\n", colorGreen+"VERIFIED"+colorReset)
					fmt.Printf("                 Traffic is routed through %s\n", expectedIP)
				} else {
					fmt.Printf("  Routing:       %s\n", colorRed+"NOT ROUTED"+colorReset)
					fmt.Printf("                 Expected: %s\n", expectedIP)
					fmt.Printf("                 Actual:   %s\n", publicIP)
					fmt.Println()
					fmt.Println("  Possible causes:")
					fmt.Println("    - VPN not connected with --route-all flag")
					fmt.Println("    - NAT not configured on VPN server")
					fmt.Println("    - Routing table not updated correctly")
				}
			} else {
				fmt.Println()
				fmt.Println("  Hint: Use --expected=<IP> to verify against VPN server IP")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&expectedIP, "expected", "", "Expected public IP (VPN server IP)")

	return cmd
}

// getPublicIP fetches the current public IP address.
func getPublicIP() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Try multiple services in case one is down
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	for _, url := range services {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip, nil
		}
	}

	return "", fmt.Errorf("could not determine public IP")
}

// ANSI color codes for log levels
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

func getLevelColor(level string) string {
	switch level {
	case "ERROR":
		return colorRed
	case "WARN":
		return colorYellow
	case "INFO":
		return colorBlue
	case "DEBUG":
		return colorGray
	default:
		return ""
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatBandwidth(bps float64) string {
	if bps < 1024 {
		return fmt.Sprintf("%.0f B/s", bps)
	}
	if bps < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bps/1024)
	}
	return fmt.Sprintf("%.1f MB/s", bps/(1024*1024))
}

func formatUptime(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%.0fm", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%.1fh", seconds/3600)
	}
	return fmt.Sprintf("%.1fd", seconds/86400)
}

func connectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect",
		Short: "Enable VPN routing (route all traffic through VPN)",
		Long: `Enable routing all traffic through the VPN connection.

This command enables the --route-all mode at runtime, routing all
internet traffic through the VPN server.

Note: The VPN node daemon must already be running in client mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.Connect()
			if err != nil {
				return err
			}

			if result.Success {
				fmt.Printf("%s VPN Connected%s\n", colorGreen, colorReset)
				fmt.Println("────────────────────────────────────────")
				fmt.Println(result.Message)
				if result.Status != nil {
					fmt.Printf("  VPN IP:    %s\n", result.Status.VPNAddress)
					fmt.Printf("  Server:    %s\n", result.Status.ServerAddr)
					fmt.Printf("  Route All: %v\n", result.Status.RouteAll)
				}
			} else {
				fmt.Printf("%s Connection Failed%s\n", colorRed, colorReset)
				fmt.Println("────────────────────────────────────────")
				fmt.Println(result.Message)
			}

			return nil
		},
	}
}

func disconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect",
		Short: "Disable VPN routing (restore direct traffic)",
		Long: `Disable routing all traffic through the VPN connection.

This command disables the --route-all mode, restoring direct internet
connectivity while keeping the VPN tunnel active.

Note: This does NOT disconnect the VPN tunnel itself, only the route-all mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.Disconnect()
			if err != nil {
				return err
			}

			if result.Success {
				fmt.Printf("%s VPN Disconnected%s\n", colorYellow, colorReset)
				fmt.Println("────────────────────────────────────────")
				fmt.Println(result.Message)
				if result.Status != nil {
					fmt.Printf("  VPN IP:    %s\n", result.Status.VPNAddress)
					fmt.Printf("  Server:    %s\n", result.Status.ServerAddr)
					fmt.Printf("  Route All: %v\n", result.Status.RouteAll)
				}
			} else {
				fmt.Printf("%s Disconnect Failed%s\n", colorRed, colorReset)
				fmt.Println("────────────────────────────────────────")
				fmt.Println(result.Message)
			}

			return nil
		},
	}
}

func connectionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "connection-status",
		Aliases: []string{"conn-status", "cs"},
		Short:   "Show VPN connection status",
		Long:    `Show the current VPN connection status including whether route-all is enabled.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			status, err := client.ConnectionStatus()
			if err != nil {
				return err
			}

			fmt.Println("\nVPN Connection Status")
			fmt.Println("────────────────────────────────────────")

			if status.Connected {
				fmt.Printf("  Status:    %sConnected%s\n", colorGreen, colorReset)
			} else {
				fmt.Printf("  Status:    %sDisconnected%s\n", colorRed, colorReset)
			}

			fmt.Printf("  VPN IP:    %s\n", status.VPNAddress)
			fmt.Printf("  Server:    %s\n", status.ServerAddr)

			if status.RouteAll {
				fmt.Printf("  Route All: %sEnabled%s (all traffic through VPN)\n", colorGreen, colorReset)
			} else {
				fmt.Printf("  Route All: %sDisabled%s (direct traffic)\n", colorYellow, colorReset)
			}

			if status.ConnectedAt != "" {
				fmt.Printf("  Since:     %s\n", status.ConnectedAt)
			}

			return nil
		},
	}
}

func sshCmd() *cobra.Command {
	var user, password string
	var execSSH bool

	cmd := &cobra.Command{
		Use:   "ssh [peer]",
		Short: "SSH to a peer via VPN",
		Long: `SSH to a peer in the VPN network.

The peer can be specified by:
  - Name (e.g., "mac-mini", "server")
  - VPN IP address (e.g., "10.8.0.1")

If no peer is specified, shows an interactive menu to select a peer.

The command will look up the peer's VPN address and construct the SSH command.
Use --exec to actually run SSH (requires sshpass to be installed).

Family password: osopanda

Examples:
  vpn ssh                         # Interactive peer selection
  vpn ssh mac-mini                # Show SSH command for mac-mini
  vpn ssh mac-mini --exec         # Actually SSH to mac-mini
  vpn ssh 10.8.0.1                # SSH to VPN IP directly
  vpn ssh server --user=root      # SSH as root to server`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Try to connect to node for peer lookup
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return fmt.Errorf("cannot connect to local node: %w", err)
			}
			defer client.Close()

			// Get network peers
			result, err := client.NetworkPeers()
			if err != nil {
				return fmt.Errorf("cannot get network peers: %w", err)
			}

			// Get our own status to filter ourselves out
			status, _ := client.Status()
			myVPNAddr := ""
			if status != nil {
				myVPNAddr = status.VPNAddress
			}

			// Filter out ourselves from the peer list
			var availablePeers []protocol.PeerListEntry
			for _, p := range result.Peers {
				if p.VPNAddress != myVPNAddr {
					availablePeers = append(availablePeers, p)
				}
			}

			if len(availablePeers) == 0 {
				fmt.Println("No other peers available in the network.")
				return nil
			}

			var target string
			if len(args) == 0 {
				// Interactive peer selection
				fmt.Println("\n" + colorGreen + "Select a peer to SSH into:" + colorReset)
				fmt.Println("────────────────────────────────────────")
				for i, p := range availablePeers {
					osInfo := ""
					if p.OS != "" {
						osInfo = fmt.Sprintf(" [%s]", p.OS)
					}
					fmt.Printf("  %d) %s (%s)%s\n", i+1, p.Name, p.VPNAddress, osInfo)
				}
				fmt.Println()
				fmt.Print("Enter number (or 'q' to quit): ")

				var input string
				fmt.Scanln(&input)
				if input == "q" || input == "" {
					return nil
				}

				var choice int
				if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 1 || choice > len(availablePeers) {
					fmt.Println("Invalid selection")
					return nil
				}

				target = availablePeers[choice-1].Name
			} else {
				target = args[0]
			}

			// Find the peer
			var targetIP string
			var targetUser string
			var peerName string

			// Check if target is already a VPN IP
			if strings.HasPrefix(target, "10.8.0.") {
				targetIP = target
				// Try to find user from peer list
				for _, p := range availablePeers {
					if p.VPNAddress == target {
						peerName = p.Name
						if p.OS == "linux" {
							targetUser = "root"
						} else {
							targetUser = p.Hostname
						}
						break
					}
				}
				if targetUser == "" {
					targetUser = user
				}
			} else {
				// Search by name
				for _, p := range availablePeers {
					if strings.EqualFold(p.Name, target) || strings.Contains(strings.ToLower(p.Name), strings.ToLower(target)) {
						targetIP = p.VPNAddress
						peerName = p.Name
						if p.OS == "linux" {
							targetUser = "root"
						} else if p.Hostname != "" {
							targetUser = p.Hostname
						} else {
							targetUser = p.Name
						}
						break
					}
				}
			}

			if targetIP == "" {
				fmt.Printf("%sPeer not found: %s%s\n", colorRed, target, colorReset)
				fmt.Println("\nAvailable peers:")
				for _, p := range availablePeers {
					fmt.Printf("  - %s (%s)\n", p.Name, p.VPNAddress)
				}
				return nil
			}

			// Override user if specified
			if user != "" {
				targetUser = user
			}
			if targetUser == "" {
				targetUser = "root" // fallback
			}

			// Override password if not specified
			if password == "" {
				password = "osopanda"
			}

			sshCmdStr := fmt.Sprintf("ssh %s@%s", targetUser, targetIP)

			if execSSH {
				// Actually execute SSH using sshpass
				fmt.Printf("\n%sConnecting to %s...%s\n\n", colorGreen, peerName, colorReset)

				// Check if sshpass is available
				if _, err := exec.LookPath("sshpass"); err != nil {
					fmt.Println("sshpass not found. Install it with: brew install hudochenkov/sshpass/sshpass")
					fmt.Println("\nAlternatively, run SSH manually:")
					fmt.Printf("  %s\n", sshCmdStr)
					fmt.Printf("  Password: %s\n", password)
					return nil
				}

				// Run sshpass with SSH
				sshCmd := exec.Command("sshpass", "-p", password, "ssh",
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile=/dev/null",
					fmt.Sprintf("%s@%s", targetUser, targetIP))
				sshCmd.Stdin = os.Stdin
				sshCmd.Stdout = os.Stdout
				sshCmd.Stderr = os.Stderr

				return sshCmd.Run()
			}

			// Just show the command
			fmt.Printf("\n%sSSH to %s%s\n", colorGreen, peerName, colorReset)
			fmt.Println("────────────────────────────────────────")
			fmt.Printf("  Peer:      %s\n", peerName)
			fmt.Printf("  VPN IP:    %s\n", targetIP)
			fmt.Printf("  User:      %s\n", targetUser)
			fmt.Printf("  Password:  %s\n", password)
			fmt.Println()
			fmt.Printf("  Command:   %s%s%s\n", colorBlue, sshCmdStr, colorReset)
			fmt.Println()
			fmt.Println("To connect directly, use --exec flag:")
			fmt.Printf("  vpn ssh %s --exec\n", target)
			fmt.Println()
			fmt.Println("Or copy the command above, or use sshpass:")
			fmt.Printf("  sshpass -p '%s' %s\n", password, sshCmdStr)

			return nil
		},
	}

	cmd.Flags().StringVar(&user, "user", "", "SSH username (auto-detected if not specified)")
	cmd.Flags().StringVar(&password, "password", "osopanda", "SSH password (default: osopanda)")
	cmd.Flags().BoolVar(&execSSH, "exec", false, "Actually execute SSH (requires sshpass)")

	return cmd
}

const cliVersion = "0.6.2"

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show CLI and node version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("VPN CLI version %s\n", cliVersion)

			// Try to get node version
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				fmt.Printf("Node version: (not connected)\n")
				return nil
			}
			defer client.Close()

			status, err := client.Status()
			if err != nil {
				fmt.Printf("Node version: (error: %v)\n", err)
				return nil
			}

			fmt.Printf("Node version: %s (%s)\n", status.Version, status.NodeName)
			return nil
		},
	}
}

func networkPeersCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:     "network-peers",
		Aliases: []string{"np", "net-peers"},
		Short:   "List all peers in the VPN network",
		Long: `List all peers known to the VPN network.

In client mode, shows peers received from the server via PEER_LIST messages.
In server mode, shows all connected clients.

Examples:
  vpn network-peers              # List all network peers
  vpn network-peers --json       # JSON output for scripting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.NetworkPeers()
			if err != nil {
				return err
			}

			if outputJSON {
				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(output))
				return nil
			}

			mode := "Client"
			if result.ServerMode {
				mode = "Server"
			}

			fmt.Printf("\nNetwork Peers (%s mode)\n", mode)
			fmt.Println("────────────────────────────────────────────────────────────")

			if len(result.Peers) == 0 {
				fmt.Println("No peers in network.")
				fmt.Println("\nNote: Peers are discovered when the server broadcasts the peer list.")
				return nil
			}

			fmt.Printf("%-20s %-15s %-25s %s\n", "NAME", "VPN IP", "HOSTNAME", "OS")
			fmt.Println("────────────────────────────────────────────────────────────")

			for _, p := range result.Peers {
				fmt.Printf("%-20s %-15s %-25s %s\n",
					p.Name, p.VPNAddress, p.Hostname, p.OS)
			}

			fmt.Println()
			fmt.Println("Use 'vpn ssh <name>' to connect to a peer.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	return cmd
}

func crashesCmd() *cobra.Command {
	var since string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:     "crashes",
		Aliases: []string{"crash", "crash-stats"},
		Short:   "Show crash statistics and last crash details",
		Long: `Show crash statistics and information about the last crash.

This command helps diagnose VPN node stability issues by showing:
- Total crashes in the time period
- How many crashes had route-all enabled
- How many times route restoration failed (which breaks internet)
- Details of the most recent crash

Examples:
  vpn crashes                    # Show stats for last 24 hours
  vpn crashes --since=-1h        # Show stats for last hour
  vpn crashes --since=-7d        # Show stats for last week
  vpn crashes --json             # JSON output for scripting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.CrashStats(since)
			if err != nil {
				return err
			}

			if outputJSON {
				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(output))
				return nil
			}

			fmt.Println("\nCrash Statistics")
			fmt.Println("────────────────────────────────────────")
			fmt.Printf("  Time Period:          %s to now\n", since)
			fmt.Printf("  Total Crashes:        %d\n", result.TotalCrashes)
			fmt.Printf("  With Route-All:       %d\n", result.CrashesWithRouteAll)

			if result.RouteRestoreFailures > 0 {
				fmt.Printf("  %sRoute Restore Fails:   %d%s (these break internet!)\n",
					colorRed, result.RouteRestoreFailures, colorReset)
			} else {
				fmt.Printf("  Route Restore Fails:  %s0%s\n", colorGreen, colorReset)
			}

			if result.LastCrash != nil {
				fmt.Println()
				fmt.Println("Last Crash/Shutdown")
				fmt.Println("────────────────────────────────────────")
				fmt.Printf("  Time:           %s\n", result.LastCrash.Timestamp)
				fmt.Printf("  Event:          %s\n", result.LastCrash.Event)
				fmt.Printf("  Reason:         %s\n", result.LastCrash.Reason)
				fmt.Printf("  Uptime:         %s\n", formatUptime(result.LastCrash.UptimeSeconds))
				fmt.Printf("  Route-All:      %v\n", result.LastCrash.RouteAll)
				if result.LastCrash.RouteAll {
					if result.LastCrash.RouteRestored {
						fmt.Printf("  Routes:         %sRestored%s\n", colorGreen, colorReset)
					} else {
						fmt.Printf("  Routes:         %sNOT RESTORED%s (internet was broken!)\n", colorRed, colorReset)
					}
				}
				fmt.Printf("  Version:        %s\n", result.LastCrash.Version)
			} else {
				fmt.Println()
				fmt.Println("No crashes recorded in this time period.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "-24h", "Time range (Splunk-like: -1h, -24h, -7d)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	return cmd
}

func lifecycleCmd() *cobra.Command {
	var limit int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:     "lifecycle",
		Aliases: []string{"events", "history"},
		Short:   "Show recent lifecycle events (starts, stops, crashes)",
		Long: `Show recent lifecycle events for the VPN node.

Events include:
- START: Node started
- STOP: Clean shutdown
- SIGNAL: Shutdown due to signal (SIGTERM, SIGINT)
- CONNECTION_LOST: Connection to server was lost
- CRASH: Unexpected termination

Examples:
  vpn lifecycle                 # Show last 20 events
  vpn lifecycle --limit=50      # Show last 50 events
  vpn lifecycle --json          # JSON output for scripting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.Lifecycle(limit)
			if err != nil {
				return err
			}

			if outputJSON {
				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(output))
				return nil
			}

			fmt.Println("\nLifecycle Events")
			fmt.Println("────────────────────────────────────────────────────────────────────────────")
			fmt.Printf("%-20s %-15s %-12s %-8s %s\n", "TIMESTAMP", "EVENT", "UPTIME", "ROUTES", "REASON")
			fmt.Println("────────────────────────────────────────────────────────────────────────────")

			for _, e := range result.Events {
				// Parse and format timestamp
				ts, _ := time.Parse(time.RFC3339, e.Timestamp)
				tsStr := ts.Format("2006-01-02 15:04:05")

				// Color the event
				eventColor := ""
				switch e.Event {
				case "START":
					eventColor = colorGreen
				case "STOP":
					eventColor = colorBlue
				case "SIGNAL":
					eventColor = colorYellow
				case "CONNECTION_LOST", "CRASH":
					eventColor = colorRed
				}

				routeStatus := "-"
				if e.RouteAll {
					if e.RouteRestored {
						routeStatus = colorGreen + "OK" + colorReset
					} else {
						routeStatus = colorRed + "FAILED" + colorReset
					}
				}

				fmt.Printf("%-20s %s%-15s%s %-12s %-8s %s\n",
					tsStr,
					eventColor, e.Event, colorReset,
					formatUptime(e.UptimeSeconds),
					routeStatus,
					truncate(e.Reason, 30))
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of events to show")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	return cmd
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// handshakeCmd sends an install handshake to the server.
func handshakeCmd() *cobra.Command {
	var (
		nodeName string
		version  string
	)

	cmd := &cobra.Command{
		Use:   "handshake",
		Short: "Send install handshake to server",
		Long: `Send an install handshake to the VPN server.

This command is typically called by install.sh after installation
to register the client with the server and test connectivity.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			// Get hostname
			hostname, _ := os.Hostname()

			// Get status for VPN address
			status, err := client.Status()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			if nodeName == "" {
				nodeName = status.NodeName
			}
			if version == "" {
				version = status.Version
			}

			// Try to get public IP
			publicIP, _ := getPublicIP()

			// Run ping test to server
			pingOK := false
			pingMS := 0
			if pingOut, err := exec.Command("ping", "-c", "1", "-W", "2", "10.8.0.1").Output(); err == nil {
				pingOK = true
				// Extract time from ping output
				if strings.Contains(string(pingOut), "time=") {
					parts := strings.Split(string(pingOut), "time=")
					if len(parts) > 1 {
						timePart := strings.Split(parts[1], " ")[0]
						var ms float64
						fmt.Sscanf(timePart, "%f", &ms)
						pingMS = int(ms)
					}
				}
			}

			// Run SSH test - try to connect to server port 22
			sshOK := false
			sshErr := ""
			conn, err := dialWithTimeout("tcp", "10.8.0.1:22", 3*time.Second)
			if err != nil {
				sshErr = err.Error()
			} else {
				sshOK = true
				conn.Close()
			}

			handshake := protocol.InstallHandshake{
				NodeName:     nodeName,
				VPNAddress:   status.VPNAddress,
				PublicIP:     publicIP,
				Hostname:     hostname,
				OS:           runtime.GOOS,
				Arch:         runtime.GOARCH,
				Version:      version,
				GoVersion:    runtime.Version(),
				InstallTS:    time.Now().Format(time.RFC3339),
				SSHTestOK:    sshOK,
				SSHTestError: sshErr,
				PingTestOK:   pingOK,
				PingTestMS:   pingMS,
			}

			result, err := client.SendHandshake(handshake)
			if err != nil {
				return err
			}

			if result.Success {
				fmt.Printf("%s Handshake Sent%s\n", colorGreen, colorReset)
				fmt.Println("────────────────────────────────────────")
				fmt.Printf("  Node:     %s\n", handshake.NodeName)
				fmt.Printf("  VPN IP:   %s\n", handshake.VPNAddress)
				fmt.Printf("  Version:  %s\n", handshake.Version)
				fmt.Printf("  Ping:     %v (%d ms)\n", handshake.PingTestOK, handshake.PingTestMS)
				fmt.Printf("  SSH:      %v\n", handshake.SSHTestOK)
				fmt.Printf("  Recorded: %v\n", result.Recorded)
				fmt.Printf("  Server:   %s\n", result.ServerVer)
			} else {
				fmt.Printf("%s Handshake Failed%s\n", colorRed, colorReset)
				fmt.Println(result.Message)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&nodeName, "name", "", "Node name (default: from status)")
	cmd.Flags().StringVar(&version, "version", "", "Version string (default: from status)")

	return cmd
}

// handshakesCmd shows handshake history.
func handshakesCmd() *cobra.Command {
	var (
		nodeName   string
		limit      int
		outputJSON bool
	)

	cmd := &cobra.Command{
		Use:   "handshakes",
		Short: "Show install handshake history",
		Long:  `Show the history of install handshakes from all clients.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.NewClient(nodeAddr)
			if err != nil {
				return err
			}
			defer client.Close()

			history, err := client.HandshakeHistory(nodeName, limit)
			if err != nil {
				return err
			}

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(history)
			}

			if len(history.Entries) == 0 {
				fmt.Println("No handshakes recorded yet.")
				return nil
			}

			fmt.Printf("Install Handshakes (%d total)\n", history.Total)
			fmt.Println("────────────────────────────────────────────────────────────────────────────")
			fmt.Printf("%-20s %-15s %-12s %-10s %-8s %-4s %-4s\n",
				"TIMESTAMP", "NODE", "VPN IP", "VERSION", "OS", "PING", "SSH")
			fmt.Println("────────────────────────────────────────────────────────────────────────────")

			for _, h := range history.Entries {
				ts, _ := time.Parse(time.RFC3339, h.Timestamp)
				tsStr := ts.Local().Format("2006-01-02 15:04")

				pingStr := colorRed + "FAIL" + colorReset
				if h.PingTestOK {
					pingStr = colorGreen + fmt.Sprintf("%dms", h.PingTestMS) + colorReset
				}

				sshStr := colorRed + "FAIL" + colorReset
				if h.SSHTestOK {
					sshStr = colorGreen + "OK" + colorReset
				}

				ver := h.Version
				if len(ver) > 7 {
					ver = ver[:7]
				}

				fmt.Printf("%-20s %-15s %-12s %-10s %-8s %-4s %-4s\n",
					tsStr,
					truncate(h.NodeName, 15),
					h.VPNAddress,
					ver,
					h.OS+"/"+h.Arch,
					pingStr,
					sshStr)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&nodeName, "filter-node", "", "Filter by node name")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	return cmd
}

func dialWithTimeout(network, addr string, timeout time.Duration) (interface{ Close() error }, error) {
	done := make(chan error, 1)
	go func() {
		c, err := exec.Command("nc", "-z", "-w", "3", strings.Split(addr, ":")[0], strings.Split(addr, ":")[1]).CombinedOutput()
		if err != nil && !strings.Contains(string(c), "succeeded") {
			done <- fmt.Errorf("connection failed")
		} else {
			done <- nil
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
		// Return a dummy closer
		return &dummyCloser{}, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("connection timeout")
	}
}

type dummyCloser struct{}

func (d *dummyCloser) Close() error { return nil }

// diagnoseCmd runs comprehensive VPN connectivity diagnostics.
func diagnoseCmd() *cobra.Command {
	var outputJSON bool
	var verbose bool

	cmd := &cobra.Command{
		Use:     "diagnose",
		Aliases: []string{"diag", "doctor", "health"},
		Short:   "Run comprehensive VPN connectivity diagnostics",
		Long: `Run a comprehensive diagnostic check on VPN connectivity.

This command performs the following checks:
  1. Local VPN node status (process running, version, uptime)
  2. VPN server reachability (ping to 10.8.0.1)
  3. Peer discovery and connectivity
  4. Routing verification (public IP check)
  5. DNS resolution test
  6. Network interface status

The output shows a summary with pass/fail status for each check,
making it easy to identify connectivity issues.

Examples:
  vpn diagnose              # Run all diagnostics
  vpn diagnose --verbose    # Show detailed output
  vpn diagnose --json       # Output as JSON for scripting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			results := runDiagnostics(nodeAddr, verbose)

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			printDiagnostics(results, verbose)
			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output")

	return cmd
}

// DiagnosticResult holds the result of a single diagnostic check.
type DiagnosticResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", "warn"
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// DiagnosticsReport holds all diagnostic results.
type DiagnosticsReport struct {
	Timestamp   string             `json:"timestamp"`
	NodeAddress string             `json:"node_address"`
	Checks      []DiagnosticResult `json:"checks"`
	Summary     struct {
		Passed int `json:"passed"`
		Failed int `json:"failed"`
		Warned int `json:"warned"`
	} `json:"summary"`
}

func runDiagnostics(nodeAddr string, verbose bool) *DiagnosticsReport {
	report := &DiagnosticsReport{
		Timestamp:   time.Now().Format(time.RFC3339),
		NodeAddress: nodeAddr,
		Checks:      []DiagnosticResult{},
	}

	// Check 1: Local node status
	report.Checks = append(report.Checks, checkLocalNode(nodeAddr))

	// Check 2: VPN server reachability
	report.Checks = append(report.Checks, checkServerPing())

	// Check 3: Peer discovery
	report.Checks = append(report.Checks, checkPeers(nodeAddr))

	// Check 4: Routing verification
	report.Checks = append(report.Checks, checkRouting())

	// Check 5: DNS resolution
	report.Checks = append(report.Checks, checkDNS())

	// Check 6: Network interface
	report.Checks = append(report.Checks, checkNetworkInterface())

	// Check 7: Internet connectivity
	report.Checks = append(report.Checks, checkInternet())

	// Calculate summary
	for _, check := range report.Checks {
		switch check.Status {
		case "pass":
			report.Summary.Passed++
		case "fail":
			report.Summary.Failed++
		case "warn":
			report.Summary.Warned++
		}
	}

	return report
}

func checkLocalNode(nodeAddr string) DiagnosticResult {
	result := DiagnosticResult{Name: "Local VPN Node"}

	client, err := cli.NewClient(nodeAddr)
	if err != nil {
		result.Status = "fail"
		result.Message = "Cannot connect to local node"
		result.Details = err.Error()
		return result
	}
	defer client.Close()

	status, err := client.Status()
	if err != nil {
		result.Status = "fail"
		result.Message = "Failed to get node status"
		result.Details = err.Error()
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("%s (v%s) - VPN IP: %s", status.NodeName, status.Version, status.VPNAddress)
	result.Details = fmt.Sprintf("Uptime: %s, Peers: %d, Traffic In: %s, Out: %s",
		status.UptimeStr, status.PeerCount,
		formatBytes(status.BytesIn), formatBytes(status.BytesOut))

	return result
}

func checkServerPing() DiagnosticResult {
	result := DiagnosticResult{Name: "VPN Server (10.8.0.1)"}

	out, err := exec.Command("ping", "-c", "2", "-W", "3", "10.8.0.1").CombinedOutput()
	if err != nil {
		result.Status = "fail"
		result.Message = "Server unreachable"
		result.Details = "Ping failed - VPN tunnel may be down"
		return result
	}

	// Extract latency
	output := string(out)
	if strings.Contains(output, "time=") {
		parts := strings.Split(output, "time=")
		if len(parts) > 1 {
			timePart := strings.Split(parts[1], " ")[0]
			result.Details = fmt.Sprintf("Latency: %s ms", timePart)
		}
	}

	result.Status = "pass"
	result.Message = "Server reachable"
	return result
}

func checkPeers(nodeAddr string) DiagnosticResult {
	result := DiagnosticResult{Name: "Peer Discovery"}

	client, err := cli.NewClient(nodeAddr)
	if err != nil {
		result.Status = "fail"
		result.Message = "Cannot connect to node"
		return result
	}
	defer client.Close()

	peers, err := client.NetworkPeers()
	if err != nil {
		result.Status = "warn"
		result.Message = "Could not get peer list"
		result.Details = err.Error()
		return result
	}

	if len(peers.Peers) == 0 {
		result.Status = "warn"
		result.Message = "No peers discovered"
		result.Details = "Peer list may not have been received yet"
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("%d peers discovered", len(peers.Peers))

	// Build peer list details
	var peerNames []string
	for _, p := range peers.Peers {
		peerNames = append(peerNames, fmt.Sprintf("%s (%s)", p.Name, p.VPNAddress))
	}
	result.Details = strings.Join(peerNames, ", ")

	return result
}

func checkRouting() DiagnosticResult {
	result := DiagnosticResult{Name: "Traffic Routing"}

	publicIP, err := getPublicIP()
	if err != nil {
		result.Status = "warn"
		result.Message = "Could not determine public IP"
		result.Details = err.Error()
		return result
	}

	// Check if routed through VPN server
	expectedIP := "95.217.238.72" // Helsinki server
	if publicIP == expectedIP {
		result.Status = "pass"
		result.Message = "Traffic routed through VPN"
		result.Details = fmt.Sprintf("Public IP: %s (Helsinki)", publicIP)
	} else {
		result.Status = "warn"
		result.Message = "Traffic NOT routed through VPN"
		result.Details = fmt.Sprintf("Public IP: %s (expected: %s)", publicIP, expectedIP)
	}

	return result
}

func checkDNS() DiagnosticResult {
	result := DiagnosticResult{Name: "DNS Resolution"}

	start := time.Now()
	out, err := exec.Command("nslookup", "google.com").CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "fail"
		result.Message = "DNS resolution failed"
		result.Details = string(out)
		return result
	}

	result.Status = "pass"
	result.Message = "DNS working"
	result.Details = fmt.Sprintf("Resolution time: %v", elapsed.Round(time.Millisecond))
	return result
}

func checkNetworkInterface() DiagnosticResult {
	result := DiagnosticResult{Name: "VPN Interface"}

	var tunName string
	if runtime.GOOS == "darwin" {
		// macOS uses utun devices
		out, err := exec.Command("sh", "-c", "ifconfig | grep -E '^utun' | head -1 | cut -d: -f1").CombinedOutput()
		if err == nil && len(out) > 0 {
			tunName = strings.TrimSpace(string(out))
		}
	} else {
		// Linux uses tun0
		tunName = "tun0"
	}

	if tunName == "" {
		result.Status = "fail"
		result.Message = "No VPN interface found"
		return result
	}

	// Check if interface is up
	out, err := exec.Command("ifconfig", tunName).CombinedOutput()
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("Interface %s not found", tunName)
		result.Details = err.Error()
		return result
	}

	output := string(out)
	if strings.Contains(output, "UP") {
		result.Status = "pass"
		result.Message = fmt.Sprintf("Interface %s is UP", tunName)

		// Extract IP if present
		if strings.Contains(output, "inet ") {
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.Contains(line, "inet ") && strings.Contains(line, "10.8.0") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						result.Details = fmt.Sprintf("IP: %s", parts[1])
					}
				}
			}
		}
	} else {
		result.Status = "fail"
		result.Message = fmt.Sprintf("Interface %s is DOWN", tunName)
	}

	return result
}

func checkInternet() DiagnosticResult {
	result := DiagnosticResult{Name: "Internet Connectivity"}

	// Try to reach a reliable external host
	start := time.Now()
	resp, err := http.Get("https://www.google.com")
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "fail"
		result.Message = "No internet connectivity"
		result.Details = err.Error()
		return result
	}
	resp.Body.Close()

	result.Status = "pass"
	result.Message = "Internet reachable"
	result.Details = fmt.Sprintf("Response time: %v", elapsed.Round(time.Millisecond))
	return result
}

func printDiagnostics(report *DiagnosticsReport, verbose bool) {
	fmt.Println()
	fmt.Println(colorBlue + "VPN Connectivity Diagnostics" + colorReset)
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Timestamp: %s\n", report.Timestamp)
	fmt.Printf("Node:      %s\n", report.NodeAddress)
	fmt.Println("───────────────────────────────────────────────────────────────")
	fmt.Println()

	for _, check := range report.Checks {
		var statusIcon, statusColor string
		switch check.Status {
		case "pass":
			statusIcon = "[PASS]"
			statusColor = colorGreen
		case "fail":
			statusIcon = "[FAIL]"
			statusColor = colorRed
		case "warn":
			statusIcon = "[WARN]"
			statusColor = colorYellow
		}

		fmt.Printf("%s%-6s%s %-20s %s\n", statusColor, statusIcon, colorReset, check.Name, check.Message)

		if verbose && check.Details != "" {
			fmt.Printf("       %s%s%s\n", colorGray, check.Details, colorReset)
		}
	}

	fmt.Println()
	fmt.Println("───────────────────────────────────────────────────────────────")
	fmt.Printf("Summary: %s%d passed%s, ", colorGreen, report.Summary.Passed, colorReset)
	if report.Summary.Failed > 0 {
		fmt.Printf("%s%d failed%s, ", colorRed, report.Summary.Failed, colorReset)
	} else {
		fmt.Printf("0 failed, ")
	}
	if report.Summary.Warned > 0 {
		fmt.Printf("%s%d warnings%s\n", colorYellow, report.Summary.Warned, colorReset)
	} else {
		fmt.Printf("0 warnings\n")
	}
	fmt.Println()

	// Print recommendations if there are failures
	if report.Summary.Failed > 0 {
		fmt.Println(colorYellow + "Recommendations:" + colorReset)
		for _, check := range report.Checks {
			if check.Status == "fail" {
				switch check.Name {
				case "Local VPN Node":
					fmt.Println("  - Check if vpn-node daemon is running: ps aux | grep vpn-node")
					fmt.Println("  - Restart the VPN service: sudo launchctl bootout/bootstrap")
				case "VPN Server (10.8.0.1)":
					fmt.Println("  - VPN server may be down - check server status")
					fmt.Println("  - Restart local VPN client to reconnect")
				case "VPN Interface":
					fmt.Println("  - VPN tunnel not established - restart VPN client")
				case "Internet Connectivity":
					fmt.Println("  - Check if route-all is enabled but VPN is disconnected")
					fmt.Println("  - Try: vpn disconnect to restore direct routing")
				case "DNS Resolution":
					fmt.Println("  - DNS may be misconfigured - check /etc/resolv.conf")
					fmt.Println("  - Try flushing DNS: sudo dscacheutil -flushcache")
				}
			}
		}
		fmt.Println()
	}
}
