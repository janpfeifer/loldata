package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LimitRule defines a rate limit rule: at most MaxRequests in a rolling Window duration.
type LimitRule struct {
	MaxRequests int
	Window      time.Duration
	Name        string
}

// Throttler manages global rate limits across multiple time windows.
// It ensures that all configured limits are strictly observed at all times.
type Throttler struct {
	mu      sync.Mutex
	rules   []LimitRule
	history [][]time.Time
	backoff time.Time
}

// NewThrottler creates a new Throttler with the specified limit rules.
func NewThrottler(rules ...LimitRule) *Throttler {
	h := make([][]time.Time, len(rules))
	for i := range rules {
		h[i] = make([]time.Time, 0, rules[i].MaxRequests)
	}
	return &Throttler{
		rules:   rules,
		history: h,
	}
}

// Acquire blocks until a request is permitted under all limit rules, or until the context is canceled.
func (t *Throttler) Acquire(ctx context.Context) error {
	for {
		t.mu.Lock()
		now := time.Now()

		// Check if we are in a forced backoff period (e.g. from HTTP 429)
		if now.Before(t.backoff) {
			wait := t.backoff.Sub(now)
			t.mu.Unlock()

			timer := time.NewTimer(wait + 5*time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}

		var maxWait time.Duration

		for i, rule := range t.rules {
			if rule.MaxRequests <= 0 {
				continue
			}
			cutoff := now.Add(-rule.Window)

			// Prune timestamps older than window
			validIdx := 0
			for validIdx < len(t.history[i]) && t.history[i][validIdx].Before(cutoff) {
				validIdx++
			}
			t.history[i] = t.history[i][validIdx:]

			// Check if limit is reached
			if len(t.history[i]) >= rule.MaxRequests {
				oldest := t.history[i][0]
				wait := oldest.Add(rule.Window).Sub(now)
				if wait > maxWait {
					maxWait = wait
				}
			}
		}

		if maxWait <= 0 {
			// Request permitted! Record timestamp in all rule histories
			for i := range t.rules {
				t.history[i] = append(t.history[i], now)
			}
			t.mu.Unlock()
			return nil
		}

		t.mu.Unlock()

		// Sleep until the earliest time a slot becomes available
		timer := time.NewTimer(maxWait + 2*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Loop back and re-check
		}
	}
}

// ForceBackoff instructs the throttler to pause all requests until the given duration has elapsed.
// Useful when receiving a 429 Too Many Requests response with a Retry-After header.
func (t *Throttler) ForceBackoff(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	target := time.Now().Add(duration)
	if target.After(t.backoff) {
		t.backoff = target
	}
}

// Stats returns a summary of current window utilization.
func (t *Throttler) Stats() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()

	var parts []string
	for i, rule := range t.rules {
		cutoff := now.Add(-rule.Window)
		count := 0
		for _, ts := range t.history[i] {
			if !ts.Before(cutoff) {
				count++
			}
		}
		name := rule.Name
		if name == "" {
			name = fmt.Sprintf("%v window", rule.Window)
		}
		parts = append(parts, fmt.Sprintf("%s: %d/%d", name, count, rule.MaxRequests))
	}
	return fmt.Sprintf("Throttler [%s]", stringsJoin(parts, ", "))
}

func stringsJoin(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	res := elems[0]
	for i := 1; i < len(elems); i++ {
		res += sep + elems[i]
	}
	return res
}
