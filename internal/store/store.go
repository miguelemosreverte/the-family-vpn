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
