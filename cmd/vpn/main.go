// vpn is the command-line interface for interacting with VPN nodes.
//
// Usage:
//
//	vpn [global flags] <command> [command flags]
//
// Commands:
//
//	status   Show node status
//	peers    List connected peers
//	update   Update node(s)
//	logs     Query logs (Splunk-like)
//	stats    Query metrics (Splunk-like)
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

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start web dashboard",
		Long: `Start a web dashboard for monitoring VPN nodes.

The dashboard provides:
  - Home: Welcome page
  - Overview: Node status, peers, bandwidth charts
  - Observability: Splunk-like log viewer and metrics charts

Examples:
  vpn ui                           # Start on http://localhost:8080
  vpn ui --listen :3000            # Start on port 3000
  vpn --node 10.8.0.1:9001 ui      # Connect to remote node`,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := ui.NewServer(nodeAddr, listenAddr)
			return server.Start()
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "localhost:8080", "Address to listen on")

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
