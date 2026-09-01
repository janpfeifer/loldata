package main

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	ref := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	defaultT := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	// Empty defaults
	got, err := ParseTime("", ref, defaultT)
	if err != nil || !got.Equal(defaultT) {
		t.Fatalf("expected default time %v, got %v (err: %v)", defaultT, got, err)
	}

	// Relative 1w
	got, err = ParseTime("1w", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := ref.Add(-7 * 24 * time.Hour)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// Relative 24h
	got, err = ParseTime("24h", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = ref.Add(-24 * time.Hour)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// Relative 7d
	got, err = ParseTime("7d", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = ref.Add(-7 * 24 * time.Hour)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// Date format YYYY-MM-DD
	got, err = ParseTime("2026-08-15", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// RFC3339
	got, err = ParseTime("2026-08-15T14:30:00Z", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = time.Date(2026, 8, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// Unix timestamp in seconds
	unixSec := int64(1755259200)
	got, err = ParseTime("1755259200", ref, defaultT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = time.Unix(unixSec, 0).UTC()
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}

	// 'all' / '0' / 'none'
	got, err = ParseTime("all", ref, defaultT)
	if err != nil || !got.IsZero() {
		t.Errorf("expected zero time for 'all', got %v (err: %v)", got, err)
	}
	got, err = ParseTime("0", ref, defaultT)
	if err != nil || !got.IsZero() {
		t.Errorf("expected zero time for '0', got %v (err: %v)", got, err)
	}
}
