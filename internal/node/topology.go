// Package node implements the VPN node daemon.
package node

import (
	"sync"
	"time"

	"github.com/miguelemosreverte/vpn/internal/protocol"
)

// NetworkNode represents a node in the mesh network.
type NetworkNode struct {
	Name        string               `json:"name"`
	VPNAddress  string               `json:"vpn_address"`
	PublicAddr  string               `json:"public_addr,omitempty"`
	OS          string               `json:"os,omitempty"`
	Version     string               `json:"version,omitempty"`
	Distance    int                  `json:"distance"`          // Hop count from us (0 = us, 1 = direct, 2+ = via relay)
	LatencyMs   float64              `json:"latency_ms"`        // RTT in milliseconds
	Bandwidth   float64              `json:"bandwidth_bps"`     // Estimated bandwidth in bytes/sec
	IsUs        bool                 `json:"is_us"`             // True if this is our node
	IsDirect    bool                 `json:"is_direct"`         // True if directly connected
	ConnectedAt time.Time            `json:"connected_at,omitempty"`
	LastSeen    time.Time            `json:"last_seen"`
	BytesIn     uint64               `json:"bytes_in"`
	BytesOut    uint64               `json:"bytes_out"`
	Geo         *protocol.GeoLocation `json:"geo,omitempty"` // Geographic location

	// Connections to other nodes (for graph visualization)
	Connections []string `json:"connections,omitempty"` // VPN addresses of connected peers
}

// NetworkEdge represents a connection between two nodes.
type NetworkEdge struct {
	From      string  `json:"from"`       // VPN address
	To        string  `json:"to"`         // VPN address
	LatencyMs float64 `json:"latency_ms"` // RTT between these two nodes
	Bandwidth float64 `json:"bandwidth_bps"`
	Direct    bool    `json:"direct"`     // Direct connection vs relayed
}

// NetworkTopology represents the full mesh network graph.
type NetworkTopology struct {
	mu    sync.RWMutex
	nodes map[string]*NetworkNode // key: VPN address
	edges map[string]*NetworkEdge // key: "from-to" sorted

	// Our identity
	ourVPNAddr string
	ourName    string
}

// NewNetworkTopology creates a new topology tracker.
func NewNetworkTopology(ourVPNAddr, ourName string) *NetworkTopology {
	t := &NetworkTopology{
		nodes:      make(map[string]*NetworkNode),
		edges:      make(map[string]*NetworkEdge),
		ourVPNAddr: ourVPNAddr,
		ourName:    ourName,
	}

	// Add ourselves
	t.nodes[ourVPNAddr] = &NetworkNode{
		Name:       ourName,
		VPNAddress: ourVPNAddr,
		Distance:   0,
		IsUs:       true,
		LastSeen:   time.Now(),
	}

	return t
}

// AddDirectPeer adds or updates a directly connected peer.
func (t *NetworkTopology) AddDirectPeer(node *NetworkNode) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node.Distance = 1
	node.IsDirect = true
	node.LastSeen = time.Now()

	// Add the node
	t.nodes[node.VPNAddress] = node

	// Add edge from us to this peer
	edgeKey := t.edgeKey(t.ourVPNAddr, node.VPNAddress)
	t.edges[edgeKey] = &NetworkEdge{
		From:      t.ourVPNAddr,
		To:        node.VPNAddress,
		LatencyMs: node.LatencyMs,
		Bandwidth: node.Bandwidth,
		Direct:    true,
	}

	// Update our connections
	if us, ok := t.nodes[t.ourVPNAddr]; ok {
		us.Connections = t.getConnectionsFor(t.ourVPNAddr)
	}
	node.Connections = append(node.Connections, t.ourVPNAddr)
}

// RemovePeer removes a peer from the topology.
func (t *NetworkTopology) RemovePeer(vpnAddr string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.nodes, vpnAddr)

	// Remove all edges involving this peer
	for key := range t.edges {
		edge := t.edges[key]
		if edge.From == vpnAddr || edge.To == vpnAddr {
			delete(t.edges, key)
		}
	}

	// Recalculate distances
	t.recalculateDistances()
}

// MergePeerTopology merges topology information received from a peer.
// This allows us to learn about nodes that are 2+ hops away.
func (t *NetworkTopology) MergePeerTopology(sourceAddr string, nodes []*NetworkNode, edges []*NetworkEdge) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Add nodes we don't know about
	for _, node := range nodes {
		if node.VPNAddress == t.ourVPNAddr {
			continue // Skip ourselves
		}

		existing, exists := t.nodes[node.VPNAddress]
		if !exists {
			// New node - add it
			node.IsDirect = false
			node.LastSeen = time.Now()
			t.nodes[node.VPNAddress] = node
		} else {
			// Update last seen
			existing.LastSeen = time.Now()
			// Keep direct connection info if we have it
			if !existing.IsDirect && node.IsDirect {
				existing.IsDirect = node.IsDirect
			}
		}
	}

	// Add edges we don't know about
	for _, edge := range edges {
		edgeKey := t.edgeKey(edge.From, edge.To)
		if _, exists := t.edges[edgeKey]; !exists {
			t.edges[edgeKey] = edge
		}
	}

	// Recalculate distances from us
	t.recalculateDistances()
}

// GetAllNodes returns all known nodes in the network.
func (t *NetworkTopology) GetAllNodes() []*NetworkNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	nodes := make([]*NetworkNode, 0, len(t.nodes))
	for _, node := range t.nodes {
		// Make a copy
		nodeCopy := *node
		nodes = append(nodes, &nodeCopy)
	}
	return nodes
}

// GetAllEdges returns all known edges in the network.
func (t *NetworkTopology) GetAllEdges() []*NetworkEdge {
	t.mu.RLock()
	defer t.mu.RUnlock()

	edges := make([]*NetworkEdge, 0, len(t.edges))
	for _, edge := range t.edges {
		edgeCopy := *edge
		edges = append(edges, &edgeCopy)
	}
	return edges
}

// GetTopologyForExport returns a snapshot suitable for sending to peers.
func (t *NetworkTopology) GetTopologyForExport() ([]*NetworkNode, []*NetworkEdge) {
	return t.GetAllNodes(), t.GetAllEdges()
}

// edgeKey creates a consistent key for an edge (sorted addresses).
func (t *NetworkTopology) edgeKey(addr1, addr2 string) string {
	if addr1 < addr2 {
		return addr1 + "-" + addr2
	}
	return addr2 + "-" + addr1
}

// getConnectionsFor returns all VPN addresses connected to the given address.
func (t *NetworkTopology) getConnectionsFor(vpnAddr string) []string {
	connections := []string{}
	for _, edge := range t.edges {
		if edge.From == vpnAddr {
			connections = append(connections, edge.To)
		} else if edge.To == vpnAddr {
			connections = append(connections, edge.From)
		}
	}
	return connections
}

// recalculateDistances uses BFS to calculate hop counts from our node.
func (t *NetworkTopology) recalculateDistances() {
	// Reset all distances to -1 (unknown)
	for _, node := range t.nodes {
		if node.VPNAddress == t.ourVPNAddr {
			node.Distance = 0
		} else {
			node.Distance = -1
		}
	}

	// BFS from our node
	visited := make(map[string]bool)
	queue := []string{t.ourVPNAddr}
	visited[t.ourVPNAddr] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentNode := t.nodes[current]
		if currentNode == nil {
			continue
		}

		// Find all neighbors
		for _, edge := range t.edges {
			var neighbor string
			if edge.From == current {
				neighbor = edge.To
			} else if edge.To == current {
				neighbor = edge.From
			} else {
				continue
			}

			if visited[neighbor] {
				continue
			}

			visited[neighbor] = true
			if neighborNode, ok := t.nodes[neighbor]; ok {
				neighborNode.Distance = currentNode.Distance + 1
				queue = append(queue, neighbor)
			}
		}
	}
}

// UpdatePeerLatency updates the latency measurement for a peer.
func (t *NetworkTopology) UpdatePeerLatency(vpnAddr string, latencyMs float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if node, ok := t.nodes[vpnAddr]; ok {
		node.LatencyMs = latencyMs
		node.LastSeen = time.Now()
	}

	// Update edge latency
	edgeKey := t.edgeKey(t.ourVPNAddr, vpnAddr)
	if edge, ok := t.edges[edgeKey]; ok {
		edge.LatencyMs = latencyMs
	}
}

// UpdatePeerStats updates traffic stats for a peer.
func (t *NetworkTopology) UpdatePeerStats(vpnAddr string, bytesIn, bytesOut uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if node, ok := t.nodes[vpnAddr]; ok {
		node.BytesIn = bytesIn
		node.BytesOut = bytesOut
		node.LastSeen = time.Now()
	}
}

// SetOurInfo updates our own node information.
func (t *NetworkTopology) SetOurInfo(name, vpnAddr, publicAddr, os, version string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ourVPNAddr = vpnAddr
	t.ourName = name

	if node, ok := t.nodes[vpnAddr]; ok {
		node.Name = name
		node.PublicAddr = publicAddr
		node.OS = os
		node.Version = version
	} else {
		t.nodes[vpnAddr] = &NetworkNode{
			Name:       name,
			VPNAddress: vpnAddr,
			PublicAddr: publicAddr,
			OS:         os,
			Version:    version,
			Distance:   0,
			IsUs:       true,
			LastSeen:   time.Now(),
		}
	}
}

// SetOurGeo updates our own node's geolocation.
func (t *NetworkTopology) SetOurGeo(geo *protocol.GeoLocation) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if node, ok := t.nodes[t.ourVPNAddr]; ok {
		node.Geo = geo
	}
}

// GetNode returns a copy of a node by VPN address, or nil if not found.
func (t *NetworkTopology) GetNode(vpnAddr string) *NetworkNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if node, ok := t.nodes[vpnAddr]; ok {
		nodeCopy := *node
		return &nodeCopy
	}
	return nil
}
