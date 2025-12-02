package node

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/miguelemosreverte/vpn/internal/protocol"
	"github.com/miguelemosreverte/vpn/internal/store"
)

// Version is set at build time via -ldflags
var Version = "dev"

// handleControlConnection processes commands from a CLI client.
func (d *Daemon) handleControlConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("[control] New connection from %s", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		var req protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			d.sendError(encoder, 0, protocol.ErrCodeInvalidParams, "invalid JSON")
			continue
		}

		d.handleRequest(encoder, &req)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[control] Connection error: %v", err)
	}
}

// handleRequest dispatches a request to the appropriate handler.
func (d *Daemon) handleRequest(enc *json.Encoder, req *protocol.Request) {
	switch req.Method {
	case "status":
		d.handleStatus(enc, req)
	case "peers":
		d.handlePeers(enc, req)
	case "update":
		d.handleUpdate(enc, req)
	case "logs":
		d.handleLogs(enc, req)
	case "stats":
		d.handleStats(enc, req)
	case "connect":
		d.handleConnect(enc, req)
	case "disconnect":
		d.handleDisconnect(enc, req)
	case "connection_status":
		d.handleConnectionStatus(enc, req)
	case "topology":
		d.handleTopology(enc, req)
	case "network_peers":
		d.handleNetworkPeers(enc, req)
	case "lifecycle":
		d.handleLifecycle(enc, req)
	case "crash_stats":
		d.handleCrashStats(enc, req)
	default:
		d.sendError(enc, req.ID, protocol.ErrCodeInvalidMethod,
			fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// handleStatus returns node status information.
func (d *Daemon) handleStatus(enc *json.Encoder, req *protocol.Request) {
	uptime := d.Uptime()
	bytesIn, bytesOut := d.Stats()

	result := protocol.StatusResult{
		NodeName:   d.config.NodeName,
		Version:    Version,
		Uptime:     uptime,
		UptimeStr:  formatDuration(uptime),
		VPNAddress: d.config.VPNAddress,
		PeerCount:  d.PeerCount(),
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
	}

	d.sendResult(enc, req.ID, result)
}

// handlePeers returns the list of connected peers.
func (d *Daemon) handlePeers(enc *json.Encoder, req *protocol.Request) {
	peers := d.GetPeers()

	peerInfos := make([]protocol.PeerInfo, len(peers))
	for i, p := range peers {
		peerInfos[i] = protocol.PeerInfo{
			Name:       p.Name,
			VPNAddress: p.VPNAddress,
			PublicIP:   p.PublicAddr,
			Connected:  p.Connected,
			BytesIn:    p.BytesIn,
			BytesOut:   p.BytesOut,
		}
	}

	d.sendResult(enc, req.ID, protocol.PeersResult{Peers: peerInfos})
}

// handleUpdate triggers a node update.
func (d *Daemon) handleUpdate(enc *json.Encoder, req *protocol.Request) {
	var params protocol.UpdateParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, "invalid params")
			return
		}
	}

	// TODO: Implement actual update logic
	// 1. git pull
	// 2. rebuild if needed
	// 3. graceful restart

	if params.All {
		// TODO: Propagate update to all peers
		log.Printf("[control] Update requested for ALL nodes (rolling=%v)", params.Rolling)
	} else {
		log.Printf("[control] Update requested for this node")
	}

	result := protocol.UpdateResult{
		Success: true,
		Updated: []string{d.config.NodeName},
	}

	d.sendResult(enc, req.ID, result)
}

// sendResult sends a successful response.
func (d *Daemon) sendResult(enc *json.Encoder, id uint64, result interface{}) {
	data, _ := json.Marshal(result)
	resp := protocol.Response{
		ID:     id,
		Result: data,
	}
	enc.Encode(resp)
}

// sendError sends an error response.
func (d *Daemon) sendError(enc *json.Encoder, id uint64, code int, message string) {
	resp := protocol.Response{
		ID: id,
		Error: &protocol.Error{
			Code:    code,
			Message: message,
		},
	}
	enc.Encode(resp)
}

// handleLogs returns logs based on Splunk-like query parameters.
func (d *Daemon) handleLogs(enc *json.Encoder, req *protocol.Request) {
	if d.store == nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, "storage not initialized")
		return
	}

	var params protocol.LogsParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, "invalid params")
			return
		}
	}

	// Default time range: last 15 minutes
	earliest := params.Earliest
	if earliest == "" {
		earliest = "-15m"
	}
	latest := params.Latest
	if latest == "" {
		latest = "now"
	}

	// Parse time range
	timeRange, err := store.ParseTimeRange(earliest, latest)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, fmt.Sprintf("invalid time range: %v", err))
		return
	}

	// Build query
	query := &store.LogQuery{
		TimeRange:  timeRange,
		Levels:     params.Levels,
		Components: params.Components,
		Search:     params.Search,
		Limit:      params.Limit,
	}
	if query.Limit <= 0 {
		query.Limit = 100
	}

	// Execute query
	result, err := d.store.QueryLogs(query)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, fmt.Sprintf("query failed: %v", err))
		return
	}

	// Convert to protocol format
	entries := make([]protocol.LogEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = protocol.LogEntry{
			ID:        e.ID,
			Timestamp: e.Timestamp.Format(time.RFC3339),
			Level:     e.Level,
			Component: e.Component,
			Message:   e.Message,
			Fields:    e.Fields,
		}
	}

	d.sendResult(enc, req.ID, protocol.LogsResult{
		Entries:    entries,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	})
}

// handleStats returns metrics based on Splunk-like query parameters.
func (d *Daemon) handleStats(enc *json.Encoder, req *protocol.Request) {
	if d.store == nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, "storage not initialized")
		return
	}

	var params protocol.StatsParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, "invalid params")
			return
		}
	}

	// Default time range: last 5 minutes
	earliest := params.Earliest
	if earliest == "" {
		earliest = "-5m"
	}
	latest := params.Latest
	if latest == "" {
		latest = "now"
	}

	// Parse time range
	timeRange, err := store.ParseTimeRange(earliest, latest)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, fmt.Sprintf("invalid time range: %v", err))
		return
	}

	// Build query
	query := &store.MetricQuery{
		TimeRange:   timeRange,
		Names:       params.Metrics,
		Granularity: params.Granularity,
	}

	// Execute query
	result, err := d.store.QueryMetrics(query)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, fmt.Sprintf("query failed: %v", err))
		return
	}

	// Convert to protocol format
	series := make([]protocol.MetricSeries, len(result.Series))
	for i, s := range result.Series {
		points := make([]protocol.MetricPoint, len(s.Points))
		for j, p := range s.Points {
			points[j] = protocol.MetricPoint{
				Timestamp:   p.Timestamp.Format(time.RFC3339),
				Name:        p.Name,
				Value:       p.Value,
				Granularity: p.Granularity,
			}
		}
		series[i] = protocol.MetricSeries{
			Name:   s.Name,
			Points: points,
		}
	}

	// Get latest values as summary
	summary := make(map[string]float64)
	if len(params.Metrics) == 0 {
		// Default metrics
		params.Metrics = []string{
			"vpn.bytes_sent", "vpn.bytes_recv",
			"vpn.packets_sent", "vpn.packets_recv",
			"vpn.active_peers", "vpn.uptime_seconds",
			"bandwidth.tx_current_bps", "bandwidth.rx_current_bps",
		}
	}
	latestValues, _ := d.store.GetLatestMetrics(params.Metrics)
	for k, v := range latestValues {
		summary[k] = v
	}

	// Get storage info
	storageInfo := make(map[string]float64)
	if stats, err := d.store.GetStorageStats(); err == nil {
		for k, v := range stats {
			if f, ok := v.(float64); ok {
				storageInfo[k] = f
			} else if i, ok := v.(int64); ok {
				storageInfo[k] = float64(i)
			}
		}
	}

	d.sendResult(enc, req.ID, protocol.StatsResult{
		Series:      series,
		Summary:     summary,
		StorageInfo: storageInfo,
	})
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// handleConnect enables route-all traffic through VPN.
func (d *Daemon) handleConnect(enc *json.Encoder, req *protocol.Request) {
	if err := d.EnableRouteAll(); err != nil {
		d.sendResult(enc, req.ID, protocol.ConnectionResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	status := d.getConnectionStatus()
	d.sendResult(enc, req.ID, protocol.ConnectionResult{
		Success: true,
		Message: "VPN routing enabled - all traffic now goes through VPN",
		Status:  status,
	})
}

// handleDisconnect disables route-all traffic through VPN.
func (d *Daemon) handleDisconnect(enc *json.Encoder, req *protocol.Request) {
	if err := d.DisableRouteAll(); err != nil {
		d.sendResult(enc, req.ID, protocol.ConnectionResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	status := d.getConnectionStatus()
	d.sendResult(enc, req.ID, protocol.ConnectionResult{
		Success: true,
		Message: "VPN routing disabled - traffic going direct",
		Status:  status,
	})
}

// handleConnectionStatus returns the current connection status.
func (d *Daemon) handleConnectionStatus(enc *json.Encoder, req *protocol.Request) {
	status := d.getConnectionStatus()
	d.sendResult(enc, req.ID, status)
}

// getConnectionStatus builds the current connection status.
func (d *Daemon) getConnectionStatus() *protocol.ConnectionStatus {
	status := &protocol.ConnectionStatus{
		Connected:  d.IsConnected(),
		RouteAll:   d.IsRouteAll(),
		VPNAddress: d.config.VPNAddress,
		ServerAddr: d.GetConnectTo(),
	}

	if status.Connected {
		status.ConnectedAt = d.startTime.Format(time.RFC3339)
	}

	return status
}

// handleTopology returns the full network topology.
// The node returns raw data; the UI/CLI layer decides how to display it.
func (d *Daemon) handleTopology(enc *json.Encoder, req *protocol.Request) {
	if d.topology == nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, "topology not initialized")
		return
	}

	nodes := d.topology.GetAllNodes()
	edges := d.topology.GetAllEdges()

	// Convert internal types to protocol types
	protoNodes := make([]*protocol.NetworkNode, len(nodes))
	for i, n := range nodes {
		protoNodes[i] = &protocol.NetworkNode{
			Name:        n.Name,
			VPNAddress:  n.VPNAddress,
			PublicAddr:  n.PublicAddr,
			OS:          n.OS,
			Version:     n.Version,
			Distance:    n.Distance,
			LatencyMs:   n.LatencyMs,
			Bandwidth:   n.Bandwidth,
			IsUs:        n.IsUs,
			IsDirect:    n.IsDirect,
			ConnectedAt: n.ConnectedAt,
			LastSeen:    n.LastSeen,
			BytesIn:     n.BytesIn,
			BytesOut:    n.BytesOut,
			Connections: n.Connections,
		}
	}

	protoEdges := make([]*protocol.NetworkEdge, len(edges))
	for i, e := range edges {
		protoEdges[i] = &protocol.NetworkEdge{
			From:      e.From,
			To:        e.To,
			LatencyMs: e.LatencyMs,
			Bandwidth: e.Bandwidth,
			Direct:    e.Direct,
		}
	}

	d.sendResult(enc, req.ID, protocol.TopologyResult{
		Nodes: protoNodes,
		Edges: protoEdges,
	})
}

// handleNetworkPeers returns the list of network peers (for client mode).
// Server mode returns connected peers, client mode returns peers from PEER_LIST.
func (d *Daemon) handleNetworkPeers(enc *json.Encoder, req *protocol.Request) {
	var peers []protocol.PeerListEntry

	if d.config.ServerMode {
		// Server mode: return connected peers
		d.mu.RLock()
		hostname, _ := os.Hostname()
		peers = make([]protocol.PeerListEntry, 0, len(d.peers)+1)

		// Add server itself first
		peers = append(peers, protocol.PeerListEntry{
			Name:       d.config.NodeName,
			VPNAddress: d.config.VPNAddress,
			Hostname:   hostname,
			OS:         "linux",
		})

		// Add connected peers
		for _, p := range d.peers {
			peers = append(peers, protocol.PeerListEntry{
				Name:       p.Name,
				VPNAddress: p.VPNAddress,
				Hostname:   p.Name,
				OS:         p.OS,
			})
		}
		d.mu.RUnlock()
	} else {
		// Client mode: return peers from PEER_LIST
		peers = d.GetNetworkPeers()
	}

	d.sendResult(enc, req.ID, protocol.NetworkPeersResult{
		Peers:      peers,
		ServerMode: d.config.ServerMode,
	})
}

// handleLifecycle returns recent lifecycle events.
func (d *Daemon) handleLifecycle(enc *json.Encoder, req *protocol.Request) {
	if d.store == nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, "storage not initialized")
		return
	}

	var params protocol.LifecycleParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, "invalid params")
			return
		}
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}

	events, err := d.store.GetLifecycleEvents(params.Limit)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, fmt.Sprintf("query failed: %v", err))
		return
	}

	// Convert to protocol format
	protoEvents := make([]protocol.LifecycleEvent, len(events))
	for i, e := range events {
		protoEvents[i] = protocol.LifecycleEvent{
			ID:            e.ID,
			Timestamp:     e.Timestamp.Format(time.RFC3339),
			Event:         e.Event,
			Reason:        e.Reason,
			UptimeSeconds: e.UptimeSeconds,
			RouteAll:      e.RouteAll,
			RouteRestored: e.RouteRestored,
			Version:       e.Version,
		}
	}

	d.sendResult(enc, req.ID, protocol.LifecycleResult{Events: protoEvents})
}

// handleCrashStats returns crash statistics.
func (d *Daemon) handleCrashStats(enc *json.Encoder, req *protocol.Request) {
	if d.store == nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, "storage not initialized")
		return
	}

	var params protocol.CrashStatsParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, "invalid params")
			return
		}
	}

	// Default: last 24 hours
	since := params.Since
	if since == "" {
		since = "-24h"
	}

	// Parse time range
	timeRange, err := store.ParseTimeRange(since, "now")
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInvalidParams, fmt.Sprintf("invalid time range: %v", err))
		return
	}

	total, withRouteAll, restoreFailures, err := d.store.GetCrashStats(timeRange.Start)
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, fmt.Sprintf("query failed: %v", err))
		return
	}

	// Get last crash
	lastCrash, err := d.store.GetLastCrash()
	if err != nil {
		d.sendError(enc, req.ID, protocol.ErrCodeInternal, fmt.Sprintf("query failed: %v", err))
		return
	}

	result := protocol.CrashStatsResult{
		TotalCrashes:         total,
		CrashesWithRouteAll:  withRouteAll,
		RouteRestoreFailures: restoreFailures,
	}

	if lastCrash != nil {
		result.LastCrash = &protocol.LifecycleEvent{
			ID:            lastCrash.ID,
			Timestamp:     lastCrash.Timestamp.Format(time.RFC3339),
			Event:         lastCrash.Event,
			Reason:        lastCrash.Reason,
			UptimeSeconds: lastCrash.UptimeSeconds,
			RouteAll:      lastCrash.RouteAll,
			RouteRestored: lastCrash.RouteRestored,
			Version:       lastCrash.Version,
		}
	}

	d.sendResult(enc, req.ID, result)
}
