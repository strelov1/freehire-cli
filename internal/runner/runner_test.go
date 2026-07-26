package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	// A runner that retried in a tight loop would hammer the server whenever it
	// is down — exactly when it can least afford it. And one that backed off
	// forever would never come back after a blip.
	// Jitter is up to 20%, so growth is asserted with that slack rather than
	// exactly — and once the cap is reached the delay is expected to wobble
	// around it, not keep climbing.
	var prev time.Duration
	for attempt := 0; attempt < 12; attempt++ {
		d := backoff(attempt)
		if d <= 0 {
			t.Fatalf("attempt %d: non-positive delay %v", attempt, d)
		}
		if d > maxBackoff {
			t.Fatalf("attempt %d: %v exceeds the cap %v", attempt, d, maxBackoff)
		}
		if attempt > 0 && d < prev && prev < maxBackoff*4/5 {
			t.Fatalf("attempt %d: delay shrank from %v to %v before reaching the cap", attempt, prev, d)
		}
		prev = d
	}
	if backoff(0) >= maxBackoff {
		t.Fatal("the first retry should be quick, not already at the cap")
	}
	if d := backoff(20); d < maxBackoff*4/5 {
		t.Fatalf("a long outage should settle near the cap, got %v", d)
	}
}

func TestServeReconnectsUntilTheContextEnds(t *testing.T) {
	// The laptop sleeps, the wifi drops, the server restarts. None of those
	// should require the user to notice and restart the runner.
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())

	err := serveLoop(ctx, func(context.Context) error {
		attempts++
		if attempts >= 3 {
			cancel()
		}
		return errors.New("connection lost")
	}, func(int) time.Duration { return time.Millisecond })

	if attempts < 3 {
		t.Fatalf("expected repeated reconnects, got %d", attempts)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stopping must be attributed to the context, got %v", err)
	}
}

func TestServeStopsImmediatelyOnAnAuthFailure(t *testing.T) {
	// Retrying a rejected token just burns the server's rate limit and hides
	// the real problem from the user.
	attempts := 0
	err := serveLoop(context.Background(), func(context.Context) error {
		attempts++
		return errFatal{errors.New("401 unauthorized")}
	}, func(int) time.Duration { return time.Millisecond })

	if attempts != 1 {
		t.Fatalf("a fatal error must not be retried, got %d attempts", attempts)
	}
	if err == nil {
		t.Fatal("the error must reach the caller")
	}
}
