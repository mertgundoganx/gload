package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

// TestStagesLinearRamp verifies that staged load ramps the worker count
// linearly within a stage (k6-style) rather than stepping instantly.
func TestStagesLinearRamp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:     srv.URL,
		Method:  "GET",
		Headers: map[string]string{},
		Timeout: 5 * time.Second,
		// RPS-capped so the test stays light and doesn't starve neighbours of
		// sockets; the ramp is asserted via the concurrency timeline, which is
		// independent of the request rate.
		Stages: []config.Stage{
			{Duration: 3 * time.Second, Target: 20, RPS: 300}, // ramp 0 -> 20
			{Duration: 2 * time.Second, Target: 20, RPS: 300}, // hold 20
			{Duration: 3 * time.Second, Target: 0, RPS: 300},  // ramp 20 -> 0
		},
	}

	r := New(cfg)
	defer r.Close()

	// Drive the metrics timeline while the test runs.
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(250 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				r.Metrics.RecordTimelinePoint()
			}
		}
	}()

	r.Run(context.Background())
	close(stop)

	tl := r.Metrics.Timeline()
	if len(tl) < 6 {
		t.Fatalf("expected a populated timeline, got %d points", len(tl))
	}

	// Find peak concurrency and when it occurred.
	peak := 0
	peakIdx := 0
	for i, p := range tl {
		if p.Concurrency > peak {
			peak = p.Concurrency
			peakIdx = i
		}
	}
	if peak < 18 {
		t.Fatalf("expected peak concurrency near 20, got %d", peak)
	}

	// Ramp evidence: an early sample should be well below the peak (proving it
	// wasn't an instant step), and a late sample should have come back down.
	early := tl[1].Concurrency
	last := tl[len(tl)-1].Concurrency
	if early >= peak {
		t.Fatalf("expected early concurrency (%d) below peak (%d) — looks like a step, not a ramp", early, peak)
	}
	if last >= peak {
		t.Fatalf("expected ramp-down: last concurrency (%d) should be below peak (%d)", last, peak)
	}
	if peakIdx == 0 {
		t.Fatalf("peak should not be at the very first sample")
	}

	snap := r.Metrics.Snapshot()
	if snap.TotalReqs == 0 {
		t.Fatal("expected requests to be sent during staged run")
	}
	t.Logf("ramp ok: peak=%d early=%d last=%d totalReqs=%d points=%d", peak, early, last, snap.TotalReqs, len(tl))
}
