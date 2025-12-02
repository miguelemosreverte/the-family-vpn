// Package node implements the VPN node daemon.
package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/miguelemosreverte/vpn/internal/protocol"
	"github.com/miguelemosreverte/vpn/internal/store"
	"github.com/miguelemosreverte/vpn/internal/tunnel"
)

// Config holds the node configuration.
type Config struct {
	NodeName      string `yaml:"name"`
	ListenVPN     string `yaml:"listen_vpn"`
	ListenWS      string `yaml:"listen_ws"`
	ListenControl string `yaml:"listen_control"`
	VPNAddress    string `yaml:"vpn_address"`
	Subnet        string `yaml:"subnet"`

	// TLS configuration
	UseTLS   bool   `yaml:"use_tls"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`

	// Encryption key (32 bytes for AES-256)
	EncryptionKey []byte `yaml:"-"`
	Encryption    bool   `yaml:"encryption"`

	// Server mode: if true, this node accepts connections and assigns IPs
	// If false, this node connects to a server
	ServerMode    bool   `yaml:"server_mode"`
	ConnectTo     string `yaml:"connect_to"` // Server address to connect to (client mode)

	// RouteAll: if true, route all traffic through VPN (client mode)
	RouteAll bool `yaml:"route_all"`

	// Data directory for SQLite storage
	DataDir string `yaml:"data_dir"`
}

// IsRoutingAllTraffic returns whether all traffic is being routed through VPN.
func (c *Config) IsRoutingAllTraffic() bool {
	return c.RouteAll
}

// Daemon is the main VPN node daemon.
type Daemon struct {
	config    Config
	startTime time.Time

	// TUN device
	tun *tunnel.TUN

	// VPN listener (server mode)
	vpnListener *tunnel.Listener

	// VPN connection (client mode)
	vpnConn *tunnel.Conn

	// Peer connections (server mode)
	peerConns   map[string]*tunnel.Conn // key: VPN IP
	peerConnsMu sync.RWMutex

	// Statistics
	mu       sync.RWMutex
	bytesIn  uint64
	bytesOut uint64
	peers    map[string]*Peer

	// Network peers (client mode - received from server via PEER_LIST)
	networkPeers   []protocol.PeerListEntry
	networkPeersMu sync.RWMutex

	// IP assignment (server mode)
	nextIP       int               // Next IP to assign (starts at 2 for 10.8.0.2)
	hostnameToIP map[string]string // Persistent IP assignment

	// Control socket
	controlListener net.Listener

	// Storage and metrics
	store            *store.Store
	metricsCollector *store.Collector
	standardMetrics  *store.StandardMetrics
	bandwidthTracker *store.BandwidthTracker

	// Network topology
	topology *NetworkTopology

	// Connection failure detection (client mode)
	connFailed     chan struct{} // Signals that VPN connection has failed
	connFailedOnce sync.Once     // Ensures we only signal failure once

	// Shutdown
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once // Ensures shutdown only runs once
}

// Peer represents a connected peer node.
type Peer struct {
	Name       string
	VPNAddress string
	PublicAddr string
	OS         string
	Connected  time.Time
	BytesIn    uint64
	BytesOut   uint64
}

// New creates a new Daemon instance.
func New(cfg Config) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		config:       cfg,
		startTime:    time.Now(),
		peers:        make(map[string]*Peer),
		peerConns:    make(map[string]*tunnel.Conn),
		hostnameToIP: make(map[string]string),
		nextIP:       2, // Start from 10.8.0.2
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() error {
	log.Printf("[node] Starting VPN node: %s", d.config.NodeName)
	log.Printf("[node] VPN Address: %s", d.config.VPNAddress)
	log.Printf("[node] Mode: %s", map[bool]string{true: "SERVER", false: "CLIENT"}[d.config.ServerMode])

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize network topology tracker
	d.topology = NewNetworkTopology(d.config.VPNAddress, d.config.NodeName)

	// Initialize storage
	if err := d.initStorage(); err != nil {
		log.Printf("[node] Warning: failed to init storage: %v (continuing without metrics)", err)
	}

	// Start control socket server
	if err := d.startControlServer(); err != nil {
		return fmt.Errorf("failed to start control server: %w", err)
	}
	log.Printf("[node] Control socket listening on %s", d.config.ListenControl)

	if d.config.ServerMode {
		// Server mode: create TUN, listen for connections
		if err := d.startServer(); err != nil {
			return fmt.Errorf("failed to start server: %w", err)
		}

		// Start deploy webhook server (on WebSocket port)
		if err := d.StartDeployServer(d.config.ListenWS); err != nil {
			log.Printf("[node] Warning: failed to start deploy server: %v", err)
		}
	} else {
		// Client mode: connect to server, then create TUN
		if err := d.startClient(); err != nil {
			return fmt.Errorf("failed to start client: %w", err)
		}
	}

	// Start metrics update goroutine
	go d.metricsLoop()

	log.Printf("[node] Node is ready")

	// Wait for shutdown signal
	select {
	case sig := <-sigCh:
		log.Printf("[node] Received signal: %v", sig)
	case <-d.ctx.Done():
		log.Printf("[node] Context cancelled")
	}

	return d.shutdown()
}

// startServer initializes server mode.
func (d *Daemon) startServer() error {
	// Create TUN device
	tunCfg := tunnel.Config{
		LocalIP:   d.config.VPNAddress,
		GatewayIP: d.config.VPNAddress, // Server is its own gateway
	}
	tun, err := tunnel.New(tunCfg)
	if err != nil {
		return fmt.Errorf("failed to create TUN: %w", err)
	}
	d.tun = tun

	// Start VPN listener
	listenCfg := tunnel.ListenConfig{
		Address:    d.config.ListenVPN,
		UseTLS:     d.config.UseTLS,
		CertFile:   d.config.CertFile,
		KeyFile:    d.config.KeyFile,
		Key:        d.config.EncryptionKey,
		Encryption: d.config.Encryption,
	}
	listener, err := tunnel.Listen(listenCfg)
	if err != nil {
		d.tun.Close()
		return fmt.Errorf("failed to start VPN listener: %w", err)
	}
	d.vpnListener = listener

	log.Printf("[node] VPN server listening on %s", d.config.ListenVPN)

	// Accept connections in background
	go d.acceptVPNConnections()

	// Route TUN packets to peers
	go d.routeTUNPackets()

	return nil
}

// startClient initializes client mode.
func (d *Daemon) startClient() error {
	// Initialize connection failure channel
	d.connFailed = make(chan struct{})

	// Connect to server
	dialCfg := tunnel.DialConfig{
		Address:    d.config.ConnectTo,
		UseTLS:     d.config.UseTLS,
		Key:        d.config.EncryptionKey,
		Encryption: d.config.Encryption,
	}
	conn, err := tunnel.Dial(dialCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	d.vpnConn = conn

	// Send handshake
	hostname, _ := os.Hostname()
	peerInfo := protocol.PeerInfo{
		Hostname: hostname,
		OS:       "darwin", // TODO: detect OS
		Version:  "0.1.0",
	}
	if err := protocol.WriteHandshake(conn.NetConn, d.config.Encryption, peerInfo); err != nil {
		conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// Read assigned IP
	assignedIP, err := protocol.ReadAssignedIP(conn.NetConn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read assigned IP: %w", err)
	}
	d.config.VPNAddress = assignedIP
	log.Printf("[node] Assigned VPN IP: %s", assignedIP)

	// Create TUN device with assigned IP
	tunCfg := tunnel.Config{
		LocalIP:   assignedIP,
		GatewayIP: tunnel.DefaultServerIP,
	}
	tun, err := tunnel.New(tunCfg)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create TUN: %w", err)
	}
	d.tun = tun

	// Route all traffic through VPN if requested
	if d.config.RouteAll {
		// Extract server IP from connect address (host:port)
		serverIP := d.config.ConnectTo
		if host, _, err := net.SplitHostPort(serverIP); err == nil {
			serverIP = host
		}
		if err := d.tun.RouteAllTraffic(serverIP); err != nil {
			log.Printf("[node] Warning: failed to route all traffic: %v", err)
		} else {
			log.Printf("[node] All traffic now routed through VPN")
		}
	}

	// Update topology with ourselves and the server
	d.topology.SetOurInfo(d.config.NodeName, assignedIP, "", "darwin", version)

	// Add server as direct peer
	// TODO: Server should send its name in the handshake response
	// For now, derive name from server address or use "server"
	serverName := "server"
	if host, _, err := net.SplitHostPort(d.config.ConnectTo); err == nil {
		// Use IP as name for now - will be replaced when server sends its name
		serverName = host
	}
	d.topology.AddDirectPeer(&NetworkNode{
		Name:       serverName,
		VPNAddress: tunnel.DefaultServerIP, // 10.8.0.1
		PublicAddr: d.config.ConnectTo,
		IsDirect:   true,
		ConnectedAt: time.Now(),
	})

	// Start packet forwarding
	go d.forwardTUNToServer()
	go d.forwardServerToTUN()

	// Start connection failure monitor (restores routes if connection drops)
	go d.monitorConnectionFailure()

	return nil
}

// acceptVPNConnections accepts incoming VPN connections (server mode).
func (d *Daemon) acceptVPNConnections() {
	for {
		conn, err := d.vpnListener.Accept()
		if err != nil {
			select {
			case <-d.ctx.Done():
				return
			default:
				log.Printf("[vpn] Accept error: %v", err)
				continue
			}
		}
		go d.handleVPNClient(conn)
	}
}

// handleVPNClient handles a connected VPN client (server mode).
func (d *Daemon) handleVPNClient(conn *tunnel.Conn) {
	remoteAddr := conn.RemoteAddr()
	log.Printf("[vpn] New client connection from %s", remoteAddr)

	// Read handshake
	encryption, peerInfo, err := protocol.ReadHandshake(conn.NetConn)
	if err != nil {
		log.Printf("[vpn] Handshake failed from %s: %v", remoteAddr, err)
		conn.Close()
		return
	}

	// Assign IP
	vpnIP := d.assignIP(peerInfo.Hostname)

	// Send assigned IP
	if err := protocol.WriteAssignedIP(conn.NetConn, vpnIP); err != nil {
		log.Printf("[vpn] Failed to send IP to %s: %v", remoteAddr, err)
		conn.Close()
		return
	}

	// Register peer
	d.mu.Lock()
	d.peers[vpnIP] = &Peer{
		Name:       peerInfo.Hostname,
		VPNAddress: vpnIP,
		PublicAddr: remoteAddr,
		OS:         peerInfo.OS,
		Connected:  time.Now(),
	}
	d.mu.Unlock()

	d.peerConnsMu.Lock()
	d.peerConns[vpnIP] = conn
	d.peerConnsMu.Unlock()

	log.Printf("[vpn] Client registered: %s (%s) -> %s (encryption: %v)",
		peerInfo.Hostname, peerInfo.OS, vpnIP, encryption)

	// Broadcast updated peer list to all clients
	d.broadcastPeerList()

	// Handle packets from this client
	d.handleClientPackets(conn, vpnIP)

	// Cleanup on disconnect
	d.mu.Lock()
	delete(d.peers, vpnIP)
	d.mu.Unlock()

	d.peerConnsMu.Lock()
	delete(d.peerConns, vpnIP)
	d.peerConnsMu.Unlock()

	// Broadcast updated peer list after disconnect
	d.broadcastPeerList()

	log.Printf("[vpn] Client disconnected: %s (%s)", peerInfo.Hostname, vpnIP)
}

// handleClientPackets reads packets from a client and writes to TUN.
func (d *Daemon) handleClientPackets(conn *tunnel.Conn, vpnIP string) {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		packet, err := conn.ReadPacket()
		if err != nil {
			log.Printf("[vpn] Read error from %s: %v", vpnIP, err)
			return
		}

		// Check for control messages
		if protocol.IsControlMessage(packet) {
			cmd := protocol.ExtractControlCommand(packet)
			log.Printf("[vpn] Control message from %s: %s", vpnIP, cmd)
			continue
		}

		// Validate IP packet
		if !tunnel.IsValidIPPacket(packet) {
			log.Printf("[vpn] Invalid packet from %s", vpnIP)
			continue
		}

		// Write to TUN (goes to kernel for routing)
		if _, err := d.tun.Write(packet); err != nil {
			log.Printf("[vpn] TUN write error: %v", err)
		}

		// Update stats
		d.mu.Lock()
		d.bytesIn += uint64(len(packet))
		if peer, ok := d.peers[vpnIP]; ok {
			peer.BytesIn += uint64(len(packet))
		}
		d.mu.Unlock()
	}
}

// routeTUNPackets reads from TUN and routes to the correct peer (server mode).
func (d *Daemon) routeTUNPackets() {
	buf := make([]byte, tunnel.MTU)

	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		n, err := d.tun.Read(buf)
		if err != nil {
			log.Printf("[tun] Read error: %v", err)
			continue
		}

		packet := buf[:n]

		// Get destination IP from packet
		destIP := tunnel.GetDestinationIP(packet)
		if destIP == nil {
			continue
		}

		destStr := destIP.String()

		// Find peer connection for this destination
		d.peerConnsMu.RLock()
		peerConn, exists := d.peerConns[destStr]
		d.peerConnsMu.RUnlock()

		if !exists {
			// Not a VPN peer, might be internet-bound (handle NAT elsewhere)
			continue
		}

		// Send to peer
		if err := peerConn.WritePacket(packet); err != nil {
			log.Printf("[tun] Failed to send to %s: %v", destStr, err)
			continue
		}

		// Update stats
		d.mu.Lock()
		d.bytesOut += uint64(len(packet))
		if peer, ok := d.peers[destStr]; ok {
			peer.BytesOut += uint64(len(packet))
		}
		d.mu.Unlock()
	}
}

// forwardTUNToServer reads from TUN and sends to server (client mode).
func (d *Daemon) forwardTUNToServer() {
	buf := make([]byte, tunnel.MTU)

	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		n, err := d.tun.Read(buf)
		if err != nil {
			log.Printf("[tun] Read error: %v", err)
			continue
		}

		if err := d.vpnConn.WritePacket(buf[:n]); err != nil {
			log.Printf("[vpn] Send error: %v", err)
			log.Printf("[vpn] Connection to server lost (send failed)")
			d.signalConnectionFailure()
			return
		}

		d.mu.Lock()
		d.bytesOut += uint64(n)
		d.mu.Unlock()
	}
}

// forwardServerToTUN reads from server and writes to TUN (client mode).
func (d *Daemon) forwardServerToTUN() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		packet, err := d.vpnConn.ReadPacket()
		if err != nil {
			log.Printf("[vpn] Read error: %v", err)
			log.Printf("[vpn] Connection to server lost (read failed)")
			d.signalConnectionFailure()
			return
		}

		// Check for control messages
		if protocol.IsControlMessage(packet) {
			cmd := protocol.ExtractControlCommand(packet)

			// Handle UPDATE_AVAILABLE from server
			if cmd == protocol.CmdUpdateAvailable {
				log.Printf("[vpn] Control message: UPDATE_AVAILABLE")
				d.HandleUpdateMessage()
				continue
			}

			// Handle PEER_LIST from server
			if protocol.IsPeerListMessage(cmd) {
				d.handlePeerListMessage(packet)
				continue
			}

			log.Printf("[vpn] Control message: %s", cmd)
			continue
		}

		// Validate and write to TUN
		if !tunnel.IsValidIPPacket(packet) {
			continue
		}

		if _, err := d.tun.Write(packet); err != nil {
			log.Printf("[tun] Write error: %v", err)
		}

		d.mu.Lock()
		d.bytesIn += uint64(len(packet))
		d.mu.Unlock()
	}
}

// assignIP assigns a VPN IP to a hostname (with persistence).
func (d *Daemon) assignIP(hostname string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if hostname already has an IP
	if ip, exists := d.hostnameToIP[hostname]; exists {
		// Verify IP is not in use
		if _, inUse := d.peers[ip]; !inUse {
			return ip
		}
	}

	// Assign new IP
	ip := fmt.Sprintf("10.8.0.%d", d.nextIP)
	d.nextIP++
	d.hostnameToIP[hostname] = ip

	return ip
}

// initStorage initializes the SQLite storage and metrics collection.
func (d *Daemon) initStorage() error {
	dataDir := d.config.DataDir
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/tmp"
		}
		dataDir = filepath.Join(homeDir, ".vpn-node")
	}

	s, err := store.New(dataDir)
	if err != nil {
		return err
	}
	d.store = s

	// Initialize metrics trackers
	d.standardMetrics = store.NewStandardMetrics()
	d.bandwidthTracker = store.NewBandwidthTracker(300) // 5 minutes of 1-second samples

	// Create metrics collector
	d.metricsCollector = store.NewCollector(d.store, time.Second)
	d.metricsCollector.RegisterSource("standard", d.standardMetrics.Source())
	d.metricsCollector.RegisterSource("bandwidth", d.bandwidthTracker.Source())
	d.metricsCollector.Start()

	// Redirect log output to store
	logWriter := store.NewLogWriter(d.store, "node", "INFO")
	log.SetOutput(store.MultiWriter(logWriter))

	log.Printf("[store] Metrics collection started (interval: 1s)")
	return nil
}

// updateMetrics updates the standard metrics with current values.
func (d *Daemon) updateMetrics() {
	if d.standardMetrics == nil {
		return
	}

	d.mu.RLock()
	bytesIn := d.bytesIn
	bytesOut := d.bytesOut
	peerCount := len(d.peers)
	d.mu.RUnlock()

	// Get packet counts from VPN connection
	var packetsSent, packetsRecv uint64
	if d.vpnConn != nil {
		_, _, packetsSent, packetsRecv = d.vpnConn.Stats()
	}

	d.standardMetrics.Update(bytesOut, bytesIn, packetsSent, packetsRecv, peerCount)
	d.bandwidthTracker.Record(bytesOut, bytesIn)
}

// metricsLoop periodically updates metrics.
func (d *Daemon) metricsLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.updateMetrics()
		}
	}
}

// shutdown gracefully stops the daemon. Safe to call multiple times.
func (d *Daemon) shutdown() error {
	d.shutdownOnce.Do(func() {
		log.Printf("[node] Shutting down...")
		d.cancel()

		// Stop metrics collection
		if d.metricsCollector != nil {
			d.metricsCollector.Stop()
		}
	})

	// These operations are idempotent, so they can be outside the Once

	if d.vpnListener != nil {
		d.vpnListener.Close()
	}

	if d.vpnConn != nil {
		d.vpnConn.Close()
	}

	// Close all peer connections
	d.peerConnsMu.Lock()
	for _, conn := range d.peerConns {
		conn.Close()
	}
	d.peerConnsMu.Unlock()

	if d.tun != nil {
		d.tun.RestoreRouting()
		d.tun.Close()
	}

	if d.controlListener != nil {
		d.controlListener.Close()
	}

	// Close storage
	if d.store != nil {
		d.store.Close()
	}

	log.Printf("[node] Shutdown complete")
	return nil
}

// startControlServer starts the control socket server.
func (d *Daemon) startControlServer() error {
	addr := d.config.ListenControl
	if addr == "" {
		addr = "127.0.0.1:9001"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	d.controlListener = listener

	go d.acceptControlConnections()
	return nil
}

// acceptControlConnections handles incoming control connections.
func (d *Daemon) acceptControlConnections() {
	for {
		conn, err := d.controlListener.Accept()
		if err != nil {
			select {
			case <-d.ctx.Done():
				return
			default:
				log.Printf("[control] Accept error: %v", err)
				continue
			}
		}
		go d.handleControlConnection(conn)
	}
}

// Uptime returns how long the daemon has been running.
func (d *Daemon) Uptime() time.Duration {
	return time.Since(d.startTime)
}

// Stats returns current statistics.
func (d *Daemon) Stats() (bytesIn, bytesOut uint64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.bytesIn, d.bytesOut
}

// PeerCount returns the number of connected peers.
func (d *Daemon) PeerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.peers)
}

// GetPeers returns a copy of all connected peers.
func (d *Daemon) GetPeers() []Peer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	peers := make([]Peer, 0, len(d.peers))
	for _, p := range d.peers {
		peers = append(peers, *p)
	}
	return peers
}

// broadcastPeerList sends the current peer list to all connected clients.
func (d *Daemon) broadcastPeerList() {
	if !d.config.ServerMode {
		return // Only server broadcasts peer lists
	}

	// Build peer list (include server itself)
	d.mu.RLock()
	peers := make([]protocol.PeerListEntry, 0, len(d.peers)+1)

	// Add server as the first peer
	hostname, _ := os.Hostname()
	peers = append(peers, protocol.PeerListEntry{
		Name:       d.config.NodeName,
		VPNAddress: d.config.VPNAddress,
		Hostname:   hostname,
		OS:         "linux",
	})

	// Add all connected clients
	for _, p := range d.peers {
		peers = append(peers, protocol.PeerListEntry{
			Name:       p.Name,
			VPNAddress: p.VPNAddress,
			Hostname:   p.Name,
			OS:         p.OS,
		})
	}
	d.mu.RUnlock()

	// Create the message
	msg := protocol.MakePeerListMessage(peers)

	// Send to all peers
	d.peerConnsMu.RLock()
	defer d.peerConnsMu.RUnlock()

	log.Printf("[vpn] Broadcasting peer list (%d peers) to %d clients", len(peers), len(d.peerConns))

	for vpnIP, conn := range d.peerConns {
		if err := conn.WritePacket(msg); err != nil {
			log.Printf("[vpn] Failed to send peer list to %s: %v", vpnIP, err)
		}
	}
}

// handlePeerListMessage processes a PEER_LIST control message (client mode).
func (d *Daemon) handlePeerListMessage(packet []byte) {
	peers, err := protocol.ParsePeerListMessage(packet)
	if err != nil {
		log.Printf("[vpn] Failed to parse peer list: %v", err)
		return
	}

	d.networkPeersMu.Lock()
	d.networkPeers = peers
	d.networkPeersMu.Unlock()

	log.Printf("[vpn] Received peer list with %d peers:", len(peers))
	for _, p := range peers {
		log.Printf("[vpn]   - %s (%s) @ %s", p.Name, p.OS, p.VPNAddress)
	}

	// Update topology with received peers
	if d.topology != nil {
		for _, p := range peers {
			// Skip ourselves
			if p.VPNAddress == d.config.VPNAddress {
				continue
			}
			d.topology.AddDirectPeer(&NetworkNode{
				Name:       p.Name,
				VPNAddress: p.VPNAddress,
				OS:         p.OS,
				IsDirect:   p.VPNAddress == "10.8.0.1", // Only server is direct
			})
		}
	}
}

// GetNetworkPeers returns the list of network peers (client mode).
func (d *Daemon) GetNetworkPeers() []protocol.PeerListEntry {
	d.networkPeersMu.RLock()
	defer d.networkPeersMu.RUnlock()

	// Return a copy
	peers := make([]protocol.PeerListEntry, len(d.networkPeers))
	copy(peers, d.networkPeers)
	return peers
}

// GetStore returns the store instance for querying logs and metrics.
func (d *Daemon) GetStore() *store.Store {
	return d.store
}

// GetBandwidth returns current and average bandwidth.
func (d *Daemon) GetBandwidth() (txCurrent, rxCurrent, txAvg, rxAvg float64) {
	if d.bandwidthTracker == nil {
		return 0, 0, 0, 0
	}
	txCurrent, rxCurrent = d.bandwidthTracker.Current()
	txAvg, rxAvg = d.bandwidthTracker.Average()
	return
}

// IsConnected returns true if VPN connection is active.
func (d *Daemon) IsConnected() bool {
	return d.vpnConn != nil
}

// IsRouteAll returns true if all traffic is routed through VPN.
func (d *Daemon) IsRouteAll() bool {
	return d.config.RouteAll
}

// EnableRouteAll enables routing all traffic through VPN.
func (d *Daemon) EnableRouteAll() error {
	if d.config.ServerMode {
		return fmt.Errorf("route-all is only supported in client mode")
	}
	if d.vpnConn == nil || d.tun == nil {
		return fmt.Errorf("VPN not connected")
	}
	if d.config.RouteAll {
		return nil // Already enabled
	}

	// Extract server IP from connect address (host:port)
	serverIP := d.config.ConnectTo
	if host, _, err := net.SplitHostPort(serverIP); err == nil {
		serverIP = host
	}

	if err := d.tun.RouteAllTraffic(serverIP); err != nil {
		return fmt.Errorf("failed to enable route-all: %w", err)
	}

	d.config.RouteAll = true
	log.Printf("[node] All traffic now routed through VPN")
	return nil
}

// DisableRouteAll disables routing all traffic through VPN.
func (d *Daemon) DisableRouteAll() error {
	if d.config.ServerMode {
		return fmt.Errorf("route-all is only supported in client mode")
	}
	if d.tun == nil {
		return fmt.Errorf("TUN device not available")
	}
	if !d.config.RouteAll {
		return nil // Already disabled
	}

	if err := d.tun.RestoreRouting(); err != nil {
		return fmt.Errorf("failed to restore routing: %w", err)
	}

	d.config.RouteAll = false
	log.Printf("[node] Traffic routing restored to direct")
	return nil
}

// GetConnectTo returns the server address for client mode.
func (d *Daemon) GetConnectTo() string {
	return d.config.ConnectTo
}

// signalConnectionFailure signals that the VPN connection has failed.
// This is called by forwarding goroutines when they encounter a fatal error.
// Safe to call multiple times - only the first call has any effect.
func (d *Daemon) signalConnectionFailure() {
	d.connFailedOnce.Do(func() {
		log.Printf("[vpn] Signaling connection failure")
		close(d.connFailed)
	})
}

// monitorConnectionFailure waits for a connection failure and restores routing.
// This ensures that if the VPN connection drops unexpectedly, the user's
// internet connectivity is restored by removing VPN routes.
func (d *Daemon) monitorConnectionFailure() {
	select {
	case <-d.ctx.Done():
		// Normal shutdown - routes will be restored in shutdown()
		return
	case <-d.connFailed:
		// Connection failed unexpectedly
		log.Printf("[vpn] ========================================")
		log.Printf("[vpn] CONNECTION FAILURE DETECTED")
		log.Printf("[vpn] ========================================")
		log.Printf("[vpn] VPN connection to server has been lost")
		log.Printf("[vpn] Restoring network routes to prevent internet loss...")

		if d.tun != nil && d.config.RouteAll {
			if err := d.tun.RestoreRouting(); err != nil {
				log.Printf("[vpn] ERROR: Failed to restore routing: %v", err)
				log.Printf("[vpn] Manual intervention may be required!")
				log.Printf("[vpn] Try: sudo route delete default; sudo route add default <your-gateway>")
			} else {
				log.Printf("[vpn] SUCCESS: Network routes restored")
				log.Printf("[vpn] Internet connectivity should be working via direct connection")
				d.config.RouteAll = false
			}
		}

		log.Printf("[vpn] ========================================")
		log.Printf("[vpn] VPN is disconnected. Restart vpn-node to reconnect.")
		log.Printf("[vpn] ========================================")

		// Trigger daemon shutdown so it exits cleanly
		d.cancel()
	}
}
