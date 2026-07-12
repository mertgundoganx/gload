package runner

import (
	"context"
	"testing"
)

func TestStartRateLimiterBufferClamp(t *testing.T) {
	// rps <= 0 yields a nil channel and a no-op stop.
	tokens, stop := startRateLimiter(context.Background(), 0)
	if tokens != nil {
		t.Fatal("expected nil tokens channel for rps <= 0")
	}
	stop() // must not panic

	// A modest rps sizes the buffer to exactly rps.
	tokens, stop = startRateLimiter(context.Background(), 1000)
	defer stop()
	if got := cap(tokens); got != 1000 {
		t.Errorf("buffer cap = %d, want 1000", got)
	}

	// An absurd rps is clamped to maxRateLimiterBuffer so it can't drive a
	// pathological allocation.
	tokens, stop = startRateLimiter(context.Background(), maxRateLimiterBuffer*5)
	defer stop()
	if got := cap(tokens); got != maxRateLimiterBuffer {
		t.Errorf("buffer cap = %d, want %d (clamped)", got, maxRateLimiterBuffer)
	}
}
