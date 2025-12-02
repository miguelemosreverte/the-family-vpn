package store

import (
	"fmt"
	"strings"
	"time"
)

// LogQuery represents a query for logs.
type LogQuery struct {
	TimeRange  *TimeRange
	Levels     []string // Filter by log levels
	Components []string // Filter by components
	Search     string   // Full-text search in message
	Limit      int      // Max results (default 1000)
	Offset     int      // Pagination offset
	Reverse    bool     // If true, oldest first; default is newest first
}

// MetricQuery represents a query for metrics.
type MetricQuery struct {
	TimeRange   *TimeRange
	Names       []string // Metric names to query
	Granularity string   // "raw", "1m", "1h", or "auto"
	Aggregation string   // "avg", "min", "max", "sum", "count" (for grouping)
	GroupBy     string   // Time grouping: "1m", "5m", "1h", etc.
}

// LogQueryResult contains query results.
type LogQueryResult struct {
	Entries    []*LogEntry `json:"entries"`
	TotalCount int64       `json:"total_count"`
	HasMore    bool        `json:"has_more"`
	Query      *LogQuery   `json:"-"`
}

// MetricQueryResult contains metric query results.
type MetricQueryResult struct {
	Series []MetricSeries `json:"series"`
	Query  *MetricQuery   `json:"-"`
}

// MetricSeries represents a time series of metric values.
type MetricSeries struct {
	Name   string        `json:"name"`
	Points []MetricPoint `json:"points"`
}

// QueryLogs queries logs with filters.
func (s *Store) QueryLogs(q *LogQuery) (*LogQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if q.Limit <= 0 {
		q.Limit = 1000
	}
	if q.Limit > 10000 {
		q.Limit = 10000
	}

	// Build query
	var conditions []string
	var args []interface{}

	if q.TimeRange != nil {
		conditions = append(conditions, "timestamp >= ? AND timestamp <= ?")
		args = append(args, q.TimeRange.Start.UnixMilli(), q.TimeRange.End.UnixMilli())
	}

	if len(q.Levels) > 0 {
		placeholders := make([]string, len(q.Levels))
		for i, level := range q.Levels {
			placeholders[i] = "?"
			args = append(args, strings.ToUpper(level))
		}
		conditions = append(conditions, fmt.Sprintf("level IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(q.Components) > 0 {
		placeholders := make([]string, len(q.Components))
		for i, comp := range q.Components {
			placeholders[i] = "?"
			args = append(args, comp)
		}
		conditions = append(conditions, fmt.Sprintf("component IN (%s)", strings.Join(placeholders, ",")))
	}

	if q.Search != "" {
		conditions = append(conditions, "message LIKE ?")
		args = append(args, "%"+q.Search+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	var totalCount int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM logs %s", whereClause)
	s.db.QueryRow(countQuery, args...).Scan(&totalCount)

	// Get results
	order := "DESC"
	if q.Reverse {
		order = "ASC"
	}

	selectQuery := fmt.Sprintf(
		"SELECT id, timestamp, level, component, message, fields FROM logs %s ORDER BY timestamp %s LIMIT ? OFFSET ?",
		whereClause, order,
	)
	args = append(args, q.Limit+1, q.Offset) // +1 to check if there are more

	rows, err := s.db.Query(selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var entries []*LogEntry
	for rows.Next() {
		var e LogEntry
		var ts int64
		var fields *string
		if err := rows.Scan(&e.ID, &ts, &e.Level, &e.Component, &e.Message, &fields); err != nil {
			continue
		}
		e.Timestamp = time.UnixMilli(ts)
		if fields != nil {
			e.Fields = *fields
		}
		entries = append(entries, &e)
	}

	hasMore := len(entries) > q.Limit
	if hasMore {
		entries = entries[:q.Limit]
	}

	return &LogQueryResult{
		Entries:    entries,
		TotalCount: totalCount,
		HasMore:    hasMore,
		Query:      q,
	}, nil
}

// QueryMetrics queries metrics with aggregation.
func (s *Store) QueryMetrics(q *MetricQuery) (*MetricQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Auto-select granularity based on time range
	granularity := q.Granularity
	if granularity == "" || granularity == "auto" {
		granularity = SuggestGranularity(q.TimeRange)
	}

	// Select appropriate table
	table := "metrics_raw"
	valueCol := "value"
	switch granularity {
	case "1m":
		table = "metrics_1m"
		valueCol = "avg_value"
	case "1h":
		table = "metrics_1h"
		valueCol = "avg_value"
	}

	result := &MetricQueryResult{
		Query: q,
	}

	// Query each metric name
	names := q.Names
	if len(names) == 0 {
		// Get all metric names
		rows, err := s.db.Query(fmt.Sprintf("SELECT DISTINCT name FROM %s", table))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			names = append(names, name)
		}
	}

	for _, name := range names {
		series := MetricSeries{Name: name}

		var rows interface{ Next() bool }
		var err error

		query := fmt.Sprintf(
			"SELECT timestamp, %s FROM %s WHERE name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
			valueCol, table,
		)
		dbRows, err := s.db.Query(query, name, q.TimeRange.Start.UnixMilli(), q.TimeRange.End.UnixMilli())
		if err != nil {
			continue
		}
		rows = dbRows
		defer dbRows.Close()

		for rows.Next() {
			var ts int64
			var value float64
			if err := dbRows.Scan(&ts, &value); err != nil {
				continue
			}
			series.Points = append(series.Points, MetricPoint{
				Timestamp:   time.UnixMilli(ts),
				Name:        name,
				Value:       value,
				Granularity: granularity,
			})
		}

		if len(series.Points) > 0 {
			result.Series = append(result.Series, series)
		}
	}

	return result, nil
}

// GetLatestMetrics returns the latest value for each metric.
func (s *Store) GetLatestMetrics(names []string) (map[string]float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]float64)

	for _, name := range names {
		var value float64
		err := s.db.QueryRow(
			"SELECT value FROM metrics_raw WHERE name = ? ORDER BY timestamp DESC LIMIT 1",
			name,
		).Scan(&value)
		if err == nil {
			result[name] = value
		}
	}

	return result, nil
}

// GetMetricStats returns statistics for a metric over a time range.
func (s *Store) GetMetricStats(name string, tr *TimeRange) (map[string]float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var minVal, maxVal, avgVal, sumVal, count float64

	err := s.db.QueryRow(`
		SELECT
			MIN(value), MAX(value), AVG(value), SUM(value), COUNT(*)
		FROM metrics_raw
		WHERE name = ? AND timestamp >= ? AND timestamp <= ?
	`, name, tr.Start.UnixMilli(), tr.End.UnixMilli()).Scan(
		&minVal, &maxVal, &avgVal, &sumVal, &count,
	)
	if err != nil {
		return nil, err
	}

	stats := map[string]float64{
		"min":   minVal,
		"max":   maxVal,
		"avg":   avgVal,
		"sum":   sumVal,
		"count": count,
	}

	return stats, nil
}

// Tail returns the latest N log entries, optionally filtered.
func (s *Store) Tail(n int, levels []string, components []string) ([]*LogEntry, error) {
	q := &LogQuery{
		Limit:      n,
		Levels:     levels,
		Components: components,
	}
	result, err := s.QueryLogs(q)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}
