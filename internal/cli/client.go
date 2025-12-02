// Package cli implements the VPN command-line interface.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/miguelemosreverte/vpn/internal/protocol"
)

// Client connects to a node's control socket.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	encoder *json.Encoder
	nextID  uint64
}

// NewClient creates a new CLI client.
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		addr = "127.0.0.1:9001"
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to node at %s: %w", addr, err)
	}

	scanner := bufio.NewScanner(conn)
	// Increase buffer size for large responses (e.g., metrics with many data points)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max

	return &Client{
		conn:    conn,
		scanner: scanner,
		encoder: json.NewEncoder(conn),
	}, nil
}

// Close closes the connection to the node.
func (c *Client) Close() error {
	return c.conn.Close()
}

// call sends a request and waits for a response.
func (c *Client) call(method string, params interface{}) (*protocol.Response, error) {
	id := atomic.AddUint64(&c.nextID, 1)

	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	req := protocol.Request{
		ID:     id,
		Method: method,
		Params: paramsJSON,
	}

	if err := c.encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("connection closed")
	}

	var resp protocol.Response
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// Status retrieves the node status.
func (c *Client) Status() (*protocol.StatusResult, error) {
	resp, err := c.call("status", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.StatusResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Peers retrieves the list of connected peers.
func (c *Client) Peers() (*protocol.PeersResult, error) {
	resp, err := c.call("peers", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.PeersResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Update triggers a node update.
func (c *Client) Update(all, rolling bool) (*protocol.UpdateResult, error) {
	params := protocol.UpdateParams{
		All:     all,
		Rolling: rolling,
	}

	resp, err := c.call("update", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.UpdateResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Logs retrieves logs with Splunk-like query parameters.
func (c *Client) Logs(params protocol.LogsParams) (*protocol.LogsResult, error) {
	resp, err := c.call("logs", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.LogsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Stats retrieves metrics with Splunk-like query parameters.
func (c *Client) Stats(params protocol.StatsParams) (*protocol.StatsResult, error) {
	resp, err := c.call("stats", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.StatsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Connect activates VPN routing (route all traffic through VPN).
func (c *Client) Connect() (*protocol.ConnectionResult, error) {
	resp, err := c.call("connect", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.ConnectionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Disconnect deactivates VPN routing (restore direct traffic).
func (c *Client) Disconnect() (*protocol.ConnectionResult, error) {
	resp, err := c.call("disconnect", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.ConnectionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// ConnectionStatus retrieves the current VPN connection state.
func (c *Client) ConnectionStatus() (*protocol.ConnectionStatus, error) {
	resp, err := c.call("connection_status", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.ConnectionStatus
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Topology retrieves the full network topology.
func (c *Client) Topology() (*protocol.TopologyResult, error) {
	resp, err := c.call("topology", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.TopologyResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// NetworkPeers retrieves the list of network peers (from PEER_LIST).
func (c *Client) NetworkPeers() (*protocol.NetworkPeersResult, error) {
	resp, err := c.call("network_peers", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.NetworkPeersResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Lifecycle retrieves recent lifecycle events.
func (c *Client) Lifecycle(limit int) (*protocol.LifecycleResult, error) {
	params := protocol.LifecycleParams{Limit: limit}
	paramsJSON, _ := json.Marshal(params)

	resp, err := c.call("lifecycle", paramsJSON)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.LifecycleResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// CrashStats retrieves crash statistics.
func (c *Client) CrashStats(since string) (*protocol.CrashStatsResult, error) {
	params := protocol.CrashStatsParams{Since: since}
	paramsJSON, _ := json.Marshal(params)

	resp, err := c.call("crash_stats", paramsJSON)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %s", resp.Error.Message)
	}

	var result protocol.CrashStatsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}
