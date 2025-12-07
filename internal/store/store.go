// Package store provides SQLite-based storage for logs and metrics with Splunk-like querying.
package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// MaxStorageBytes is the maximum storage size (50MB)
	MaxStorageBytes = 50 * 1024 * 1024

	// MetricsRetentionRaw is how long to keep raw metrics (1 hour)
	MetricsRetentionRaw = 1 * time.Hour

	// MetricsRetention1m is how long to keep 1-minute aggregates (24 hours)
	MetricsRetention1m = 24 * time.Hour

	// MetricsRetention1h is how long to keep 1-hour aggregates (30 days)
	MetricsRetention1h = 30 * 24 * time.Hour

	// LogsRetention is default log retention (7 days, subject to size limit)
	LogsRetention = 7 * 24 * time.Hour
)

// Store manages SQLite storage for logs and metrics.
type Store struct {
	db        *sql.DB
	dbPath    string
	mu        sync.RWMutex
	stopChan  chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once // Ensures Close only runs once

	// Subscribers for real-time streaming
	logSubs   map[chan *LogEntry]struct{}
	logSubsMu sync.RWMutex
}

// LogEntry represents a single log entry.
type LogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // DEBUG, INFO, WARN, ERROR
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Fields    string    `json:"fields,omitempty"` // JSON-encoded extra fields
}

// MetricPoint represents a single metric data point.
type MetricPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Name        string    `json:"name"`
	Value       float64   `json:"value"`
	Tags        string    `json:"tags,omitempty"` // JSON-encoded tags
	Granularity string    `json:"granularity"`    // raw, 1m, 1h
}

// New creates a new Store instance.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "vpn.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Store{
		db:       db,
		dbPath:   dbPath,
		stopChan: make(chan struct{}),
		logSubs:  make(map[chan *LogEntry]struct{}),
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	// Start background maintenance
	s.wg.Add(1)
	go s.maintenanceLoop()

	log.Printf("[store] Initialized SQLite store at %s", dbPath)
	return s, nil
}

func (s *Store) initSchema() error {
	schema := `
	-- Logs table
	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,  -- Unix timestamp in milliseconds
		level TEXT NOT NULL,
		component TEXT NOT NULL,
		message TEXT NOT NULL,
		fields TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
	CREATE INDEX IF NOT EXISTS idx_logs_component ON logs(component);

	-- Raw metrics (high resolution, short retention)
	CREATE TABLE IF NOT EXISTS metrics_raw (
		timestamp INTEGER NOT NULL,  -- Unix timestamp in milliseconds
		name TEXT NOT NULL,
		value REAL NOT NULL,
		tags TEXT,
		PRIMARY KEY (timestamp, name)
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_raw_name ON metrics_raw(name);

	-- 1-minute aggregated metrics
	CREATE TABLE IF NOT EXISTS metrics_1m (
		timestamp INTEGER NOT NULL,  -- Unix timestamp (minute boundary)
		name TEXT NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sum_value REAL NOT NULL,
		count INTEGER NOT NULL,
		tags TEXT,
		PRIMARY KEY (timestamp, name)
	);

	-- 1-hour aggregated metrics
	CREATE TABLE IF NOT EXISTS metrics_1h (
		timestamp INTEGER NOT NULL,  -- Unix timestamp (hour boundary)
		name TEXT NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sum_value REAL NOT NULL,
		count INTEGER NOT NULL,
		tags TEXT,
		PRIMARY KEY (timestamp, name)
	);

	-- Storage metadata
	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT
	);

	-- Lifecycle events (start, stop, crash)
	CREATE TABLE IF NOT EXISTS lifecycle (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		event TEXT NOT NULL,      -- START, STOP, CRASH, SIGNAL
		reason TEXT,              -- Detailed reason or signal name
		uptime_seconds REAL,      -- How long the node was running
		route_all INTEGER,        -- Was route-all enabled (1/0)
		route_restored INTEGER,   -- Were routes restored successfully (1/0)
		version TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_lifecycle_timestamp ON lifecycle(timestamp);
	CREATE INDEX IF NOT EXISTS idx_lifecycle_event ON lifecycle(event);

	-- Install handshakes (tracked per client install)
	CREATE TABLE IF NOT EXISTS handshakes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,    -- When the handshake was received
		node_name TEXT NOT NULL,       -- Client node name
		vpn_address TEXT,              -- VPN IP assigned
		public_ip TEXT,                -- Client's public IP
		hostname TEXT,                 -- Client's hostname
		os TEXT,                       -- Operating system (darwin, linux)
		arch TEXT,                     -- Architecture (amd64, arm64)
		version TEXT,                  -- Git commit hash
		go_version TEXT,               -- Go version used to build
		install_ts TEXT,               -- When install.sh ran
		ssh_test_ok INTEGER,           -- SSH test passed (1/0)
		ssh_test_error TEXT,           -- SSH test error message
		ping_test_ok INTEGER,          -- Ping test passed (1/0)
		ping_test_ms INTEGER           -- Ping latency in ms
	);
	CREATE INDEX IF NOT EXISTS idx_handshakes_timestamp ON handshakes(timestamp);
	CREATE INDEX IF NOT EXISTS idx_handshakes_node_name ON handshakes(node_name);

	-- ==========================================================================
	-- Connection Intent Protocol: Client State Tracking
	-- ==========================================================================
	-- This table tracks the connection state of each client as seen by the server.
	-- Used to determine whether a client should receive a RECONNECT_INVITE after
	-- server restart.
	--
	-- The protocol:
	-- 1. When client connects with routing enabled, server records state=connected_routing
	-- 2. When client sends DISCONNECT_INTENT, server updates state=disconnected_intentional
	-- 3. After server restart, clients with state=connected_routing (no intent) get invited
	-- 4. Clients with state=disconnected_intentional are NOT invited (user chose to disconnect)
	-- ==========================================================================
	CREATE TABLE IF NOT EXISTS client_states (
		vpn_address TEXT PRIMARY KEY,          -- Client's VPN IP (unique identifier)
		node_name TEXT NOT NULL,               -- Client's node name
		state TEXT NOT NULL,                   -- connected_routing, connected_no_routing, disconnected_intentional
		route_all INTEGER NOT NULL DEFAULT 0,  -- Was routing enabled (1/0)
		connected_at INTEGER,                  -- When client connected (unix ms)
		disconnected_at INTEGER,               -- When client disconnected (unix ms)
		disconnect_reason TEXT,                -- Reason for disconnect if intentional
		last_updated INTEGER NOT NULL          -- Last state update timestamp
	);
	CREATE INDEX IF NOT EXISTS idx_client_states_state ON client_states(state);
	`
	_, err := s.db.Exec(schema)
	return err
}

// WriteLog writes a log entry.
func (s *Store) WriteLog(level, component, message, fields string) error {
	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Component: component,
		Message:   message,
		Fields:    fields,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		"INSERT INTO logs (timestamp, level, component, message, fields) VALUES (?, ?, ?, ?, ?)",
		entry.Timestamp.UnixMilli(), level, component, message, fields,
	)
	if err != nil {
		return err
	}

	// Notify subscribers
	s.notifyLogSubscribers(entry)
	return nil
}

// WriteMetric writes a metric data point.
func (s *Store) WriteMetric(name string, value float64, tags string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO metrics_raw (timestamp, name, value, tags) VALUES (?, ?, ?, ?)",
		time.Now().UnixMilli(), name, value, tags,
	)
	return err
}

// WriteBatchMetrics writes multiple metrics at once.
func (s *Store) WriteBatchMetrics(metrics []MetricPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR REPLACE INTO metrics_raw (timestamp, name, value, tags) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range metrics {
		if _, err := stmt.Exec(m.Timestamp.UnixMilli(), m.Name, m.Value, m.Tags); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SubscribeLogs returns a channel for real-time log streaming.
func (s *Store) SubscribeLogs() chan *LogEntry {
	ch := make(chan *LogEntry, 100)
	s.logSubsMu.Lock()
	s.logSubs[ch] = struct{}{}
	s.logSubsMu.Unlock()
	return ch
}

// UnsubscribeLogs removes a log subscription.
func (s *Store) UnsubscribeLogs(ch chan *LogEntry) {
	s.logSubsMu.Lock()
	delete(s.logSubs, ch)
	s.logSubsMu.Unlock()
	close(ch)
}

func (s *Store) notifyLogSubscribers(entry *LogEntry) {
	s.logSubsMu.RLock()
	defer s.logSubsMu.RUnlock()

	for ch := range s.logSubs {
		select {
		case ch <- entry:
		default:
			// Drop if buffer full
		}
	}
}

// Close closes the store. Safe to call multiple times.
func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.stopChan)
		s.wg.Wait()
		err = s.db.Close()
	})
	return err
}

func (s *Store) maintenanceLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	aggregateTicker := time.NewTicker(1 * time.Minute)
	defer aggregateTicker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.enforceRetention()
			s.enforceStorageLimit()
		case <-aggregateTicker.C:
			s.aggregateMetrics()
		}
	}
}

func (s *Store) enforceRetention() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Delete old raw metrics
	cutoff := now.Add(-MetricsRetentionRaw).UnixMilli()
	s.db.Exec("DELETE FROM metrics_raw WHERE timestamp < ?", cutoff)

	// Delete old 1m aggregates
	cutoff = now.Add(-MetricsRetention1m).UnixMilli()
	s.db.Exec("DELETE FROM metrics_1m WHERE timestamp < ?", cutoff)

	// Delete old 1h aggregates
	cutoff = now.Add(-MetricsRetention1h).UnixMilli()
	s.db.Exec("DELETE FROM metrics_1h WHERE timestamp < ?", cutoff)

	// Delete old logs
	cutoff = now.Add(-LogsRetention).UnixMilli()
	s.db.Exec("DELETE FROM logs WHERE timestamp < ?", cutoff)
}

func (s *Store) enforceStorageLimit() {
	// Get current DB size
	info, err := os.Stat(s.dbPath)
	if err != nil {
		return
	}

	if info.Size() < MaxStorageBytes {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[store] Storage limit reached (%d bytes), evicting old data", info.Size())

	// Delete oldest 20% of logs
	s.db.Exec(`
		DELETE FROM logs WHERE id IN (
			SELECT id FROM logs ORDER BY timestamp ASC LIMIT (SELECT COUNT(*) / 5 FROM logs)
		)
	`)

	// Vacuum to reclaim space
	s.db.Exec("VACUUM")
}

func (s *Store) aggregateMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Aggregate raw -> 1m (for data older than 1 minute)
	minuteAgo := now.Add(-1 * time.Minute).Truncate(time.Minute)
	s.db.Exec(`
		INSERT OR REPLACE INTO metrics_1m (timestamp, name, min_value, max_value, avg_value, sum_value, count, tags)
		SELECT
			(timestamp / 60000) * 60000 as ts_minute,
			name,
			MIN(value),
			MAX(value),
			AVG(value),
			SUM(value),
			COUNT(*),
			tags
		FROM metrics_raw
		WHERE timestamp < ?
		GROUP BY ts_minute, name, tags
	`, minuteAgo.UnixMilli())

	// Aggregate 1m -> 1h (for data older than 1 hour)
	hourAgo := now.Add(-1 * time.Hour).Truncate(time.Hour)
	s.db.Exec(`
		INSERT OR REPLACE INTO metrics_1h (timestamp, name, min_value, max_value, avg_value, sum_value, count, tags)
		SELECT
			(timestamp / 3600000) * 3600000 as ts_hour,
			name,
			MIN(min_value),
			MAX(max_value),
			SUM(avg_value * count) / SUM(count),
			SUM(sum_value),
			SUM(count),
			tags
		FROM metrics_1m
		WHERE timestamp < ?
		GROUP BY ts_hour, name, tags
	`, hourAgo.UnixMilli())
}

// LifecycleEvent represents a node lifecycle event (start, stop, crash).
type LifecycleEvent struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Event          string    `json:"event"`           // START, STOP, CRASH, SIGNAL
	Reason         string    `json:"reason"`          // Detailed reason or signal name
	UptimeSeconds  float64   `json:"uptime_seconds"`  // How long the node was running
	RouteAll       bool      `json:"route_all"`       // Was route-all enabled
	RouteRestored  bool      `json:"route_restored"`  // Were routes restored successfully
	Version        string    `json:"version"`
}

// WriteLifecycleEvent records a lifecycle event.
func (s *Store) WriteLifecycleEvent(event, reason string, uptimeSeconds float64, routeAll, routeRestored bool, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	routeAllInt := 0
	if routeAll {
		routeAllInt = 1
	}
	routeRestoredInt := 0
	if routeRestored {
		routeRestoredInt = 1
	}

	_, err := s.db.Exec(
		"INSERT INTO lifecycle (timestamp, event, reason, uptime_seconds, route_all, route_restored, version) VALUES (?, ?, ?, ?, ?, ?, ?)",
		time.Now().UnixMilli(), event, reason, uptimeSeconds, routeAllInt, routeRestoredInt, version,
	)
	return err
}

// GetLifecycleEvents returns recent lifecycle events.
func (s *Store) GetLifecycleEvents(limit int) ([]LifecycleEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, timestamp, event, reason, uptime_seconds, route_all, route_restored, version
		FROM lifecycle
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []LifecycleEvent
	for rows.Next() {
		var e LifecycleEvent
		var tsMs int64
		var routeAllInt, routeRestoredInt int
		var reason, version sql.NullString
		if err := rows.Scan(&e.ID, &tsMs, &e.Event, &reason, &e.UptimeSeconds, &routeAllInt, &routeRestoredInt, &version); err != nil {
			return nil, err
		}
		e.Timestamp = time.UnixMilli(tsMs)
		e.Reason = reason.String
		e.Version = version.String
		e.RouteAll = routeAllInt == 1
		e.RouteRestored = routeRestoredInt == 1
		events = append(events, e)
	}
	return events, nil
}

// GetLastCrash returns the most recent crash event.
func (s *Store) GetLastCrash() (*LifecycleEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT id, timestamp, event, reason, uptime_seconds, route_all, route_restored, version
		FROM lifecycle
		WHERE event IN ('CRASH', 'SIGNAL', 'CONNECTION_LOST')
		ORDER BY timestamp DESC
		LIMIT 1
	`)

	var e LifecycleEvent
	var tsMs int64
	var routeAllInt, routeRestoredInt int
	var reason, version sql.NullString
	if err := row.Scan(&e.ID, &tsMs, &e.Event, &reason, &e.UptimeSeconds, &routeAllInt, &routeRestoredInt, &version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No crash found
		}
		return nil, err
	}
	e.Timestamp = time.UnixMilli(tsMs)
	e.Reason = reason.String
	e.Version = version.String
	e.RouteAll = routeAllInt == 1
	e.RouteRestored = routeRestoredInt == 1
	return &e, nil
}

// GetCrashStats returns crash statistics for a time period.
func (s *Store) GetCrashStats(since time.Time) (total int, withRouteAll int, routeRestoreFailures int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sinceMs := since.UnixMilli()

	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM lifecycle
		WHERE event IN ('CRASH', 'SIGNAL', 'CONNECTION_LOST')
		AND timestamp >= ?
	`, sinceMs).Scan(&total)
	if err != nil {
		return
	}

	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM lifecycle
		WHERE event IN ('CRASH', 'SIGNAL', 'CONNECTION_LOST')
		AND route_all = 1
		AND timestamp >= ?
	`, sinceMs).Scan(&withRouteAll)
	if err != nil {
		return
	}

	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM lifecycle
		WHERE event IN ('CRASH', 'SIGNAL', 'CONNECTION_LOST')
		AND route_all = 1
		AND route_restored = 0
		AND timestamp >= ?
	`, sinceMs).Scan(&routeRestoreFailures)
	return
}

// GetStorageStats returns storage statistics.
func (s *Store) GetStorageStats() (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})

	info, err := os.Stat(s.dbPath)
	if err == nil {
		stats["db_size_bytes"] = info.Size()
		stats["db_size_mb"] = float64(info.Size()) / (1024 * 1024)
	}

	var count int64
	s.db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&count)
	stats["log_count"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM metrics_raw").Scan(&count)
	stats["metrics_raw_count"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM metrics_1m").Scan(&count)
	stats["metrics_1m_count"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM metrics_1h").Scan(&count)
	stats["metrics_1h_count"] = count

	return stats, nil
}

// HandshakeRecord represents a stored install handshake.
type HandshakeRecord struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	NodeName     string    `json:"node_name"`
	VPNAddress   string    `json:"vpn_address"`
	PublicIP     string    `json:"public_ip"`
	Hostname     string    `json:"hostname"`
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	Version      string    `json:"version"`
	GoVersion    string    `json:"go_version"`
	InstallTS    string    `json:"install_ts"`
	SSHTestOK    bool      `json:"ssh_test_ok"`
	SSHTestError string    `json:"ssh_test_error"`
	PingTestOK   bool      `json:"ping_test_ok"`
	PingTestMS   int       `json:"ping_test_ms"`
}

// WriteHandshake records an install handshake from a client.
func (s *Store) WriteHandshake(nodeName, vpnAddress, publicIP, hostname, osName, arch, version, goVersion, installTS string, sshTestOK bool, sshTestError string, pingTestOK bool, pingTestMS int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sshOK := 0
	if sshTestOK {
		sshOK = 1
	}
	pingOK := 0
	if pingTestOK {
		pingOK = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO handshakes (timestamp, node_name, vpn_address, public_ip, hostname, os, arch, version, go_version, install_ts, ssh_test_ok, ssh_test_error, ping_test_ok, ping_test_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UnixMilli(), nodeName, vpnAddress, publicIP, hostname, osName, arch, version, goVersion, installTS, sshOK, sshTestError, pingOK, pingTestMS,
	)
	return err
}

// GetHandshakeHistory returns handshake history, optionally filtered by node name.
func (s *Store) GetHandshakeHistory(nodeName string, limit int) ([]HandshakeRecord, int, error) {
	if limit <= 0 {
		limit = 100
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var query string
	var args []interface{}

	if nodeName != "" {
		query = `
			SELECT id, timestamp, node_name, vpn_address, public_ip, hostname, os, arch, version, go_version, install_ts, ssh_test_ok, ssh_test_error, ping_test_ok, ping_test_ms
			FROM handshakes
			WHERE node_name = ?
			ORDER BY timestamp DESC
			LIMIT ?`
		args = []interface{}{nodeName, limit}
	} else {
		query = `
			SELECT id, timestamp, node_name, vpn_address, public_ip, hostname, os, arch, version, go_version, install_ts, ssh_test_ok, ssh_test_error, ping_test_ok, ping_test_ms
			FROM handshakes
			ORDER BY timestamp DESC
			LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []HandshakeRecord
	for rows.Next() {
		var r HandshakeRecord
		var tsMs int64
		var sshOK, pingOK int
		var vpnAddr, pubIP, hostname, osName, arch, version, goVersion, installTS, sshErr sql.NullString

		if err := rows.Scan(&r.ID, &tsMs, &r.NodeName, &vpnAddr, &pubIP, &hostname, &osName, &arch, &version, &goVersion, &installTS, &sshOK, &sshErr, &pingOK, &r.PingTestMS); err != nil {
			return nil, 0, err
		}

		r.Timestamp = time.UnixMilli(tsMs)
		r.VPNAddress = vpnAddr.String
		r.PublicIP = pubIP.String
		r.Hostname = hostname.String
		r.OS = osName.String
		r.Arch = arch.String
		r.Version = version.String
		r.GoVersion = goVersion.String
		r.InstallTS = installTS.String
		r.SSHTestOK = sshOK == 1
		r.SSHTestError = sshErr.String
		r.PingTestOK = pingOK == 1

		records = append(records, r)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM handshakes"
	if nodeName != "" {
		countQuery += " WHERE node_name = ?"
		s.db.QueryRow(countQuery, nodeName).Scan(&total)
	} else {
		s.db.QueryRow(countQuery).Scan(&total)
	}

	return records, total, nil
}

// =============================================================================
// Connection Intent Protocol: Client State Management
// =============================================================================

// ClientState represents the connection state of a client as tracked by the server.
type ClientState struct {
	VPNAddress       string     `json:"vpn_address"`
	NodeName         string     `json:"node_name"`
	State            string     `json:"state"` // connected_routing, connected_no_routing, disconnected_intentional
	RouteAll         bool       `json:"route_all"`
	ConnectedAt      *time.Time `json:"connected_at,omitempty"`
	DisconnectedAt   *time.Time `json:"disconnected_at,omitempty"`
	DisconnectReason string     `json:"disconnect_reason,omitempty"`
	LastUpdated      time.Time  `json:"last_updated"`
}

// Client state constants
const (
	ClientStateConnectedRouting   = "connected_routing"    // Connected with routing enabled
	ClientStateConnectedNoRouting = "connected_no_routing" // Connected without routing
	ClientStateDisconnectedIntent = "disconnected_intentional" // User requested disconnect
)

// SetClientConnected records that a client has connected.
// Called by the server when a client establishes a VPN connection.
func (s *Store) SetClientConnected(vpnAddress, nodeName string, routeAll bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	state := ClientStateConnectedNoRouting
	if routeAll {
		state = ClientStateConnectedRouting
	}
	routeAllInt := 0
	if routeAll {
		routeAllInt = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO client_states (vpn_address, node_name, state, route_all, connected_at, last_updated)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vpn_address) DO UPDATE SET
			node_name = excluded.node_name,
			state = excluded.state,
			route_all = excluded.route_all,
			connected_at = excluded.connected_at,
			disconnected_at = NULL,
			disconnect_reason = NULL,
			last_updated = excluded.last_updated
	`, vpnAddress, nodeName, state, routeAllInt, now, now)
	return err
}

// SetClientRouteAll updates whether a client has routing enabled.
// Called when client enables/disables VPN routing while connected.
func (s *Store) SetClientRouteAll(vpnAddress string, routeAll bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	state := ClientStateConnectedNoRouting
	if routeAll {
		state = ClientStateConnectedRouting
	}
	routeAllInt := 0
	if routeAll {
		routeAllInt = 1
	}

	_, err := s.db.Exec(`
		UPDATE client_states
		SET state = ?, route_all = ?, last_updated = ?
		WHERE vpn_address = ?
	`, state, routeAllInt, now, vpnAddress)
	return err
}

// SetClientDisconnectedIntentional records that a client intentionally disconnected.
// Called when server receives DISCONNECT_INTENT from a client.
func (s *Store) SetClientDisconnectedIntentional(vpnAddress, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

	_, err := s.db.Exec(`
		UPDATE client_states
		SET state = ?, disconnected_at = ?, disconnect_reason = ?, last_updated = ?
		WHERE vpn_address = ?
	`, ClientStateDisconnectedIntent, now, reason, now, vpnAddress)
	return err
}

// GetClientsForReconnectInvite returns clients that should receive a RECONNECT_INVITE.
// These are clients that:
// 1. Were connected with routing enabled (state = connected_routing)
// 2. Did NOT send a DISCONNECT_INTENT (state != disconnected_intentional)
// This is called after server restart to determine which clients to invite back.
func (s *Store) GetClientsForReconnectInvite() ([]ClientState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT vpn_address, node_name, state, route_all, connected_at, disconnected_at, disconnect_reason, last_updated
		FROM client_states
		WHERE state = ?
	`, ClientStateConnectedRouting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []ClientState
	for rows.Next() {
		var c ClientState
		var routeAllInt int
		var connectedAt, disconnectedAt, lastUpdated sql.NullInt64
		var disconnectReason sql.NullString

		if err := rows.Scan(&c.VPNAddress, &c.NodeName, &c.State, &routeAllInt, &connectedAt, &disconnectedAt, &disconnectReason, &lastUpdated); err != nil {
			return nil, err
		}

		c.RouteAll = routeAllInt == 1
		if connectedAt.Valid {
			t := time.UnixMilli(connectedAt.Int64)
			c.ConnectedAt = &t
		}
		if disconnectedAt.Valid {
			t := time.UnixMilli(disconnectedAt.Int64)
			c.DisconnectedAt = &t
		}
		c.DisconnectReason = disconnectReason.String
		if lastUpdated.Valid {
			c.LastUpdated = time.UnixMilli(lastUpdated.Int64)
		}

		clients = append(clients, c)
	}

	return clients, nil
}

// GetClientState returns the current state of a specific client.
func (s *Store) GetClientState(vpnAddress string) (*ClientState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT vpn_address, node_name, state, route_all, connected_at, disconnected_at, disconnect_reason, last_updated
		FROM client_states
		WHERE vpn_address = ?
	`, vpnAddress)

	var c ClientState
	var routeAllInt int
	var connectedAt, disconnectedAt, lastUpdated sql.NullInt64
	var disconnectReason sql.NullString

	if err := row.Scan(&c.VPNAddress, &c.NodeName, &c.State, &routeAllInt, &connectedAt, &disconnectedAt, &disconnectReason, &lastUpdated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	c.RouteAll = routeAllInt == 1
	if connectedAt.Valid {
		t := time.UnixMilli(connectedAt.Int64)
		c.ConnectedAt = &t
	}
	if disconnectedAt.Valid {
		t := time.UnixMilli(disconnectedAt.Int64)
		c.DisconnectedAt = &t
	}
	c.DisconnectReason = disconnectReason.String
	if lastUpdated.Valid {
		c.LastUpdated = time.UnixMilli(lastUpdated.Int64)
	}

	return &c, nil
}

// ClearAllClientStates resets all client states (used during server shutdown/restart).
func (s *Store) ClearAllClientStates() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Don't delete - just mark as disconnected without intent (so they can be invited back)
	// Clients that sent DISCONNECT_INTENT keep their state
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`
		UPDATE client_states
		SET disconnected_at = ?, last_updated = ?
		WHERE state != ?
	`, now, now, ClientStateDisconnectedIntent)
	return err
}
