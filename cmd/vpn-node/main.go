// vpn-node is the VPN node daemon.
//
// Usage:
//
//	vpn-node [flags]
//
// Server mode (accepts connections):
//
//	sudo vpn-node --server --vpn-addr 10.8.0.1 --listen-vpn :8443
//
// Client mode (connects to server):
//
//	sudo vpn-node --connect 95.217.238.72:8443
//
// The node daemon runs continuously, maintaining VPN tunnels and WebSocket
// connections to other nodes in the mesh network.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/miguelemosreverte/vpn/internal/node"
)

func main() {
	// Flags
	name := flag.String("name", "", "Node name (default: hostname)")
	vpnAddr := flag.String("vpn-addr", "10.8.0.1", "VPN IP address for this node")
	listenVPN := flag.String("listen-vpn", ":8443", "VPN listener address (server mode)")
	listenWS := flag.String("listen-ws", ":9000", "WebSocket listener address")
	listenControl := flag.String("listen-control", "127.0.0.1:9001", "Control socket address")

	// Mode flags
	serverMode := flag.Bool("server", false, "Run in server mode (accept connections)")
	connectTo := flag.String("connect", "", "Server address to connect to (client mode)")

	// TLS flags
	useTLS := flag.Bool("tls", false, "Use TLS encryption for VPN connections")
	certFile := flag.String("cert", "certs/server.crt", "TLS certificate file")
	keyFile := flag.String("key", "certs/server.key", "TLS private key file")

	// Encryption flag
	encryption := flag.Bool("encrypt", true, "Enable packet encryption (AES-256-GCM)")

	// Routing flags - route-all defaults to true for VPN clients
	routeAll := flag.Bool("route-all", true, "Route all traffic through VPN (client mode, enabled by default)")
	noRouteAll := flag.Bool("no-route-all", false, "Disable routing all traffic through VPN (direct mode)")

	flag.Parse()

	// If --no-route-all is explicitly set, override route-all
	if *noRouteAll {
		*routeAll = false
	}

	// Validate mode
	if !*serverMode && *connectTo == "" {
		fmt.Println("Error: must specify either --server or --connect <address>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  Server mode: sudo vpn-node --server --vpn-addr 10.8.0.1")
		fmt.Println("  Client mode: sudo vpn-node --connect 95.217.238.72:8443")
		os.Exit(1)
	}

	// Check for root/admin (required for TUN device)
	if os.Getuid() != 0 {
		fmt.Println("Warning: VPN requires root privileges to create TUN device")
		fmt.Println("Run with: sudo vpn-node ...")
	}

	// Default name to hostname
	nodeName := *name
	// If name is empty or looks like an unexpanded shell variable, use actual hostname
	if nodeName == "" || nodeName == "$(hostname)" || nodeName == "$HOSTNAME" {
		hostname, err := os.Hostname()
		if err != nil {
			nodeName = "unknown"
		} else {
			nodeName = hostname
		}
	}

	// Encryption key (in production, use proper key exchange)
	encryptionKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256

	cfg := node.Config{
		NodeName:      nodeName,
		VPNAddress:    *vpnAddr,
		ListenVPN:     *listenVPN,
		ListenWS:      *listenWS,
		ListenControl: *listenControl,
		ServerMode:    *serverMode,
		ConnectTo:     *connectTo,
		UseTLS:        *useTLS,
		CertFile:      *certFile,
		KeyFile:       *keyFile,
		Encryption:    *encryption,
		EncryptionKey: encryptionKey,
		RouteAll:      *routeAll,
	}

	mode := "CLIENT"
	if cfg.ServerMode {
		mode = "SERVER"
	}

	fmt.Printf(`
╔═══════════════════════════════════════════════════╗
║              VPN NODE DAEMON                       ║
╠═══════════════════════════════════════════════════╣
║  Name:       %-36s ║
║  Mode:       %-36s ║
║  VPN IP:     %-36s ║
║  OS:         %-36s ║
║  Encryption: %-36v ║
║  TLS:        %-36v ║
╚═══════════════════════════════════════════════════╝
`, cfg.NodeName, mode, cfg.VPNAddress, runtime.GOOS, cfg.Encryption, cfg.UseTLS)

	if cfg.ServerMode {
		fmt.Printf("  Listening on: %s (VPN), %s (WS), %s (Control)\n\n",
			cfg.ListenVPN, cfg.ListenWS, cfg.ListenControl)
	} else {
		fmt.Printf("  Connecting to: %s\n\n", cfg.ConnectTo)
	}

	daemon := node.New(cfg)
	if err := daemon.Run(); err != nil {
		log.Fatalf("daemon error: %v", err)
	}
}
