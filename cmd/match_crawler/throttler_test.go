package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestThrottlerSingleRule(t *testing.T) {
	// 5 requests per 100ms
	throttler := NewThrottler(LimitRule{
		MaxRequests: 5,
		Window:      100 * time.Millisecond,
		Name:        "100ms",
	})

	ctx := context.Background()
	start := time.Now()

	// Acquire 5 tokens immediately
	for i := 0; i < 5; i++ {
		if err := throttler.Acquire(ctx); err != nil {
			t.Fatalf("unexpected error on acquire %d: %v", i, err)
		}
	}

	firstBatchDuration := time.Since(start)
	if firstBatchDuration > 50*time.Millisecond {
		t.Errorf("expected first 5 requests to be fast, took %v", firstBatchDuration)
	}

	// 6th acquire should block until 100ms window rolls
	if err := throttler.Acquire(ctx); err != nil {
		t.Fatalf("unexpected error on acquire 6: %v", err)
	}
	totalDuration := time.Since(start)
	if totalDuration < 90*time.Millisecond {
		t.Errorf("expected 6th request to wait ~100ms, took %v", totalDuration)
	}
}

func TestThrottlerMultiRule(t *testing.T) {
	// Rule 1: 3 reqs per 50ms
	// Rule 2: 5 reqs per 200ms
	throttler := NewThrottler(
		LimitRule{MaxRequests: 3, Window: 50 * time.Millisecond, Name: "50ms"},
		LimitRule{MaxRequests: 5, Window: 200 * time.Millisecond, Name: "200ms"},
	)

	ctx := context.Background()
	start := time.Now()

	// Acquire 3 immediately (rule 1 limit reached)
	for i := 0; i < 3; i++ {
		if err := throttler.Acquire(ctx); err != nil {
			t.Fatalf("unexpected error on acquire %d: %v", i, err)
		}
	}

	// 4th will wait for 50ms rule
	if err := throttler.Acquire(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d4 := time.Since(start)
	if d4 < 45*time.Millisecond {
		t.Errorf("expected 4th request to wait for rule 1 (~50ms), took %v", d4)
	}

	// 5th request
	if err := throttler.Acquire(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 6th request should hit rule 2 (max 5 per 200ms)
	if err := throttler.Acquire(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d6 := time.Since(start)
	if d6 < 190*time.Millisecond {
		t.Errorf("expected 6th request to wait for rule 2 (~200ms), took %v", d6)
	}
}

func TestThrottlerContextCancel(t *testing.T) {
	throttler := NewThrottler(LimitRule{MaxRequests: 1, Window: 1 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	if err := throttler.Acquire(ctx); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := throttler.Acquire(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	wg.Wait()
}
