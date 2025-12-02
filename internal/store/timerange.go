package store

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeRange represents a time range for queries.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// ParseTimeRange parses Splunk-like time range specifications.
//
// Supported formats:
//
// Relative time modifiers (snap to unit with @):
//   - -1h           = 1 hour ago
//   - -30m          = 30 minutes ago
//   - -7d           = 7 days ago
//   - -1w           = 1 week ago
//   - -1h@h         = 1 hour ago, snapped to hour boundary
//   - -1d@d         = 1 day ago, snapped to day boundary
//   - @h            = beginning of current hour
//   - @d            = beginning of current day
//
// Absolute time:
//   - 2024-01-15                    = midnight on date
//   - 2024-01-15T14:30:00           = specific time
//   - 2024-01-15T14:30:00Z          = UTC time
//   - 1704067200                    = Unix timestamp (seconds)
//
// Special keywords:
//   - now           = current time
//   - today         = midnight today
//   - yesterday     = midnight yesterday
//
// Time range (two values):
//   - "-1h" to "now"
//   - "-24h" to "-1h"
//   - "2024-01-15" to "2024-01-16"
func ParseTimeRange(earliest, latest string) (*TimeRange, error) {
	now := time.Now()

	start, err := parseTimeSpec(earliest, now)
	if err != nil {
		return nil, fmt.Errorf("invalid earliest time '%s': %w", earliest, err)
	}

	end, err := parseTimeSpec(latest, now)
	if err != nil {
		return nil, fmt.Errorf("invalid latest time '%s': %w", latest, err)
	}

	if start.After(end) {
		return nil, fmt.Errorf("earliest time (%s) is after latest time (%s)", start, end)
	}

	return &TimeRange{Start: start, End: end}, nil
}

// ParseRelativeTime parses a single relative time specification.
func ParseRelativeTime(spec string) (time.Time, error) {
	return parseTimeSpec(spec, time.Now())
}

func parseTimeSpec(spec string, now time.Time) (time.Time, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))

	if spec == "" || spec == "now" {
		return now, nil
	}

	// Special keywords
	switch spec {
	case "today":
		return truncateToDay(now), nil
	case "yesterday":
		return truncateToDay(now.Add(-24 * time.Hour)), nil
	}

	// Unix timestamp (all digits, 10+ chars for seconds, 13+ for millis)
	if isNumeric(spec) {
		ts, _ := strconv.ParseInt(spec, 10, 64)
		if ts > 1e12 {
			// Milliseconds
			return time.UnixMilli(ts), nil
		}
		return time.Unix(ts, 0), nil
	}

	// Relative time: -1h, -30m, -7d, etc.
	if strings.HasPrefix(spec, "-") || strings.HasPrefix(spec, "+") {
		return parseRelative(spec, now)
	}

	// Snap to boundary: @h, @d, @w, @m
	if strings.HasPrefix(spec, "@") {
		return snapToBoundary(now, spec[1:])
	}

	// ISO date/datetime
	return parseAbsolute(spec)
}

// Relative time regex: -1h, +30m, -7d@d, etc.
var relativeRe = regexp.MustCompile(`^([+-])(\d+)([smhdwMy])(?:@([smhdwMy]))?$`)

func parseRelative(spec string, now time.Time) (time.Time, error) {
	matches := relativeRe.FindStringSubmatch(spec)
	if matches == nil {
		return time.Time{}, fmt.Errorf("invalid relative time format")
	}

	sign := matches[1]
	amount, _ := strconv.Atoi(matches[2])
	unit := matches[3]
	snap := matches[4]

	if sign == "-" {
		amount = -amount
	}

	t := addDuration(now, amount, unit)

	// Apply snap if specified
	if snap != "" {
		var err error
		t, err = snapToBoundary(t, snap)
		if err != nil {
			return time.Time{}, err
		}
	}

	return t, nil
}

func addDuration(t time.Time, amount int, unit string) time.Time {
	switch unit {
	case "s":
		return t.Add(time.Duration(amount) * time.Second)
	case "m":
		return t.Add(time.Duration(amount) * time.Minute)
	case "h":
		return t.Add(time.Duration(amount) * time.Hour)
	case "d":
		return t.AddDate(0, 0, amount)
	case "w":
		return t.AddDate(0, 0, amount*7)
	case "M":
		return t.AddDate(0, amount, 0)
	case "y":
		return t.AddDate(amount, 0, 0)
	default:
		return t
	}
}

func snapToBoundary(t time.Time, unit string) (time.Time, error) {
	switch unit {
	case "s":
		return t.Truncate(time.Second), nil
	case "m":
		return t.Truncate(time.Minute), nil
	case "h":
		return t.Truncate(time.Hour), nil
	case "d":
		return truncateToDay(t), nil
	case "w":
		// Snap to Monday
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return truncateToDay(t.AddDate(0, 0, -(weekday - 1))), nil
	case "M":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()), nil
	case "y":
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location()), nil
	default:
		return time.Time{}, fmt.Errorf("unknown snap unit: %s", unit)
	}
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func parseAbsolute(spec string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"01/02/2006",
		"01/02/2006 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, spec); err == nil {
			return t, nil
		}
	}

	// Try local timezone
	for _, format := range formats {
		if t, err := time.ParseInLocation(format, spec, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized time format")
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours() / 24)
	h := int(d.Hours()) % 24
	if h > 0 {
		return fmt.Sprintf("%dd%dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}

// SuggestGranularity suggests the best metric granularity for a time range.
func SuggestGranularity(tr *TimeRange) string {
	duration := tr.End.Sub(tr.Start)

	if duration <= time.Hour {
		return "raw"
	}
	if duration <= 24*time.Hour {
		return "1m"
	}
	return "1h"
}
