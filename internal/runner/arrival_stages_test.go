package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

// TestArrivalStagesRamp verifies the open model ramps the arrival rate: the
// measured throughput should climb toward the target during the ramp-up stage,
// hold near it, and fall during ramp-down.
func TestArrivalStagesRamp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:       srv.URL,
		Method:    "GET",
		Headers:   map[string]string{},
		Timeout:   5 * time.Second,
		OpenModel: true,
		Stages: []config.Stage{
			{Duration: 3 * time.Second, Target: 200}, // ramp 0 -> 200 req/s
			{Duration: 2 * time.Second, Target: 200}, // hold 200 req/s
			{Duration: 3 * time.Second, Target: 0},   // ramp 200 -> 0
		},
	}
	r := New(cfg)
	defer r.Close()

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

	peak := 0.0
	early := tl[1].RPS
	for _, p := range tl {
		if p.RPS > peak {
			peak = p.RPS
		}
	}
	last := tl[len(tl)-1].RPS

	t.Logf("arrival ramp: early=%.0f peak=%.0f last=%.0f rps, points=%d", early, peak, last, len(tl))

	// Peak throughput should approach the 200 req/s target (allow slack for the
	// loopback server sharing CPU).
	if peak < 130 {
		t.Fatalf("expected peak arrival throughput near 200 rps, got %.0f", peak)
	}
	// It should have ramped: early well below peak, and come back down at the end.
	if early >= peak {
		t.Fatalf("expected early rps (%.0f) below peak (%.0f) — looks like a step, not a ramp", early, peak)
	}
	if last >= peak {
		t.Fatalf("expected ramp-down: last rps (%.0f) below peak (%.0f)", last, peak)
	}

	snap := r.Metrics.Snapshot()
	if snap.TotalReqs == 0 {
		t.Fatal("expected requests during arrival-stage run")
	}
}
