// Package protocol defines the wire protocols for VPN communication.
package protocol

import (
	"encoding/json"
	"time"
)

// Control protocol messages between CLI and Node daemon.

// Request represents a CLI request to the node.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response represents a node response to the CLI.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error represents an error response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// StatusResult is returned by the "status" method.
type StatusResult struct {
	NodeName       string        `json:"node_name"`
	Version        string        `json:"version"`
	Uptime         time.Duration `json:"uptime"`
	UptimeStr      string        `json:"uptime_str"`
	VPNAddress     string        `json:"vpn_address"`
	PeerCount      int           `json:"peer_count"`
	BytesIn        uint64        `json:"bytes_in"`
	BytesOut       uint64        `json:"bytes_out"`
	ServerMode     bool          `json:"server_mode"`     // True if this is a server node
	ReconnectCount int           `json:"reconnect_count"` // Number of reconnections this session
}

// PeerInfo represents a connected peer.
type PeerInfo struct {
	Hostname   string       `json:"hostname"`
	Name       string       `json:"name"`
	VPNAddress string       `json:"vpn_address"`
	PublicIP   string       `json:"public_ip,omitempty"`
	OS         string       `json:"os,omitempty"`
	Version    string       `json:"version,omitempty"`
	Connected  time.Time    `json:"connected"`
	BytesIn    uint64       `json:"bytes_in"`
	BytesOut   uint64       `json:"bytes_out"`
	Latency    string       `json:"latency,omitempty"`
	Bandwidth  float64      `json:"bandwidth_bps,omitempty"`
	Geo        *GeoLocation `json:"geo,omitempty"`
}

// PeersResult is returned by the "peers" method.
type PeersResult struct {
	Peers []PeerInfo `json:"peers"`
}

// NetworkNode represents a node in the mesh network topology.
type NetworkNode struct {
	Name        string       `json:"name"`
	VPNAddress  string       `json:"vpn_address"`
	PublicAddr  string       `json:"public_addr,omitempty"`
	OS          string       `json:"os,omitempty"`
	Version     string       `json:"version,omitempty"`
	Distance    int          `json:"distance"`      // Hop count (0 = us, 1 = direct, 2+ = via relay)
	LatencyMs   float64      `json:"latency_ms"`    // RTT in milliseconds
	Bandwidth   float64      `json:"bandwidth_bps"` // Estimated bandwidth
	IsUs        bool         `json:"is_us"`         // True if this is our node
	IsDirect    bool         `json:"is_direct"`     // True if directly connected
	ConnectedAt time.Time    `json:"connected_at,omitempty"`
	LastSeen    time.Time    `json:"last_seen"`
	BytesIn     uint64       `json:"bytes_in"`
	BytesOut    uint64       `json:"bytes_out"`
	Connections []string     `json:"connections,omitempty"` // VPN addresses of connected peers
	Geo         *GeoLocation `json:"geo,omitempty"`
}

// NetworkEdge represents a connection between two nodes in the topology.
type NetworkEdge struct {
	From      string  `json:"from"`       // VPN address
	To        string  `json:"to"`         // VPN address
	LatencyMs float64 `json:"latency_ms"` // RTT between these two nodes
	Bandwidth float64 `json:"bandwidth_bps"`
	Direct    bool    `json:"direct"` // Direct connection vs relayed
}

// TopologyResult is returned by the "topology" method.
type TopologyResult struct {
	Nodes []*NetworkNode `json:"nodes"`
	Edges []*NetworkEdge `json:"edges"`
}

// UpdateParams are parameters for the "update" method.
type UpdateParams struct {
	All     bool `json:"all,omitempty"`
	Rolling bool `json:"rolling,omitempty"`
}

// UpdateResult is returned by the "update" method.
type UpdateResult struct {
	Success bool     `json:"success"`
	Updated []string `json:"updated"` // List of node names updated
	Errors  []string `json:"errors,omitempty"`
}

// LogsParams are parameters for the "logs" method.
type LogsParams struct {
	Earliest   string   `json:"earliest,omitempty"`   // Splunk-like: -1h, -30m, @d
	Latest     string   `json:"latest,omitempty"`     // Splunk-like: now, -5m
	Levels     []string `json:"levels,omitempty"`     // DEBUG, INFO, WARN, ERROR
	Components []string `json:"components,omitempty"` // conn, tun, node, etc.
	Search     string   `json:"search,omitempty"`     // Full-text search
	Limit      int      `json:"limit,omitempty"`      // Max results
	Follow     bool     `json:"follow,omitempty"`     // Real-time streaming
}

// LogEntry represents a single log entry.
type LogEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Message   string `json:"message"`
	Fields    string `json:"fields,omitempty"`
}

// LogsResult is returned by the "logs" method.
type LogsResult struct {
	Entries    []LogEntry `json:"entries"`
	TotalCount int64      `json:"total_count"`
	HasMore    bool       `json:"has_more"`
}

// StatsParams are parameters for the "stats" method.
type StatsParams struct {
	Earliest    string   `json:"earliest,omitempty"`    // Time range start
	Latest      string   `json:"latest,omitempty"`      // Time range end
	Metrics     []string `json:"metrics,omitempty"`     // Metric names to query
	Granularity string   `json:"granularity,omitempty"` // raw, 1m, 1h, auto
}

// MetricPoint represents a single metric data point.
type MetricPoint struct {
	Timestamp   string  `json:"timestamp"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Granularity string  `json:"granularity"`
}

// MetricSeries represents a time series of metric values.
type MetricSeries struct {
	Name   string        `json:"name"`
	Points []MetricPoint `json:"points"`
}

// StatsResult is returned by the "stats" method.
type StatsResult struct {
	Series      []MetricSeries     `json:"series"`
	Summary     map[string]float64 `json:"summary,omitempty"`     // Latest values
	StorageInfo map[string]float64 `json:"storage_info,omitempty"` // DB stats
}

// ConnectionStatus represents the current VPN connection state.
type ConnectionStatus struct {
	Connected   bool   `json:"connected"`
	VPNAddress  string `json:"vpn_address,omitempty"`
	ServerAddr  string `json:"server_addr,omitempty"`
	RouteAll    bool   `json:"route_all"`
	ConnectedAt string `json:"connected_at,omitempty"`
}

// ConnectionResult is returned by connect/disconnect methods.
type ConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Status  *ConnectionStatus `json:"status,omitempty"`
}

// NetworkPeersResult is returned by the "network_peers" method.
type NetworkPeersResult struct {
	Peers      []PeerListEntry `json:"peers"`
	ServerMode bool            `json:"server_mode"`
}

// LifecycleEvent represents a node lifecycle event (start, stop, crash).
type LifecycleEvent struct {
	ID             int64   `json:"id"`
	Timestamp      string  `json:"timestamp"`
	Event          string  `json:"event"`           // START, STOP, CRASH, SIGNAL, CONNECTION_LOST
	Reason         string  `json:"reason"`          // Detailed reason
	UptimeSeconds  float64 `json:"uptime_seconds"`  // How long the node was running
	RouteAll       bool    `json:"route_all"`       // Was route-all enabled
	RouteRestored  bool    `json:"route_restored"`  // Were routes restored successfully
	Version        string  `json:"version"`
}

// LifecycleParams are parameters for the "lifecycle" method.
type LifecycleParams struct {
	Limit int `json:"limit,omitempty"` // Max events to return
}

// LifecycleResult is returned by the "lifecycle" method.
type LifecycleResult struct {
	Events []LifecycleEvent `json:"events"`
}

// CrashStatsParams are parameters for the "crash_stats" method.
type CrashStatsParams struct {
	Since string `json:"since,omitempty"` // Time range: -1h, -24h, -7d
}

// CrashStatsResult is returned by the "crash_stats" method.
type CrashStatsResult struct {
	TotalCrashes        int              `json:"total_crashes"`
	CrashesWithRouteAll int              `json:"crashes_with_route_all"`
	RouteRestoreFailures int             `json:"route_restore_failures"`
	LastCrash           *LifecycleEvent  `json:"last_crash,omitempty"`
}

// Common error codes.
const (
	ErrCodeInvalidMethod = -32601
	ErrCodeInvalidParams = -32602
	ErrCodeInternal      = -32603
)
