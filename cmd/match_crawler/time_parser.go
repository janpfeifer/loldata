package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTime parses a human-friendly time string, relative duration, date string, or unix timestamp.
// If input is empty, defaultValue is returned.
// Relative durations (e.g., "1w", "7d", "24h", "30m") are evaluated relative to refTime (subtracted if positive).
func ParseTime(input string, refTime time.Time, defaultValue time.Time) (time.Time, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	lower := strings.ToLower(input)
	if lower == "all" || lower == "none" || lower == "0" || lower == "unlimited" {
		return time.Time{}, nil
	}

	// 1. Try unix timestamp (seconds or milliseconds)
	if n, err := strconv.ParseInt(input, 10, 64); err == nil {
		if n > 1e11 { // milliseconds
			return time.UnixMilli(n).UTC(), nil
		}
		return time.Unix(n, 0).UTC(), nil
	}

	// 2. Try relative duration (e.g., "1w", "7d", "24h", "10m", "30s")
	if dur, ok := parseRelativeDuration(input); ok {
		if dur > 0 {
			return refTime.Add(-dur).UTC(), nil
		}
		return refTime.Add(dur).UTC(), nil
	}

	// 3. Try standard date/time layouts
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
		time.DateOnly,
		time.DateTime,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, input); err == nil {
			return t.UTC(), nil
		}
		if t, err := time.ParseInLocation(layout, input, time.Local); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time value %q (supported formats: '7d', '1w', '24h', 'YYYY-MM-DD', 'YYYY-MM-DDTHH:MM:SSZ', unix timestamp)", input)
}

func parseRelativeDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	// Check if ends with 'd' (days), 'w' (weeks), 'y' (years)
	multiplier := time.Duration(1)
	switch {
	case strings.HasSuffix(s, "w") || strings.HasSuffix(s, "W"):
		multiplier = 7 * 24 * time.Hour
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "d") || strings.HasSuffix(s, "D"):
		multiplier = 24 * time.Hour
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "y") || strings.HasSuffix(s, "Y"):
		multiplier = 365 * 24 * time.Hour
		s = s[:len(s)-1]
	default:
		// Try standard Go duration (h, m, s, etc.)
		if d, err := time.ParseDuration(s); err == nil {
			return d, true
		}
		return 0, false
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}

	return time.Duration(val * float64(multiplier)), true
}
