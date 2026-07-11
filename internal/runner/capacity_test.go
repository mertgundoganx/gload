package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

// TestCapacityProbeFindsKnee runs the probe against a target with a known
// capacity ceiling: a semaphore of `slots` concurrent handlers, each taking
// `work`. Max sustainable throughput ≈ slots/work. Beyond `slots` concurrent
// clients, requests queue and latency climbs while RPS plateaus — so the probe
// should detect a knee around `slots` and report MaxRPS near the ceiling.
func TestCapacityProbeFindsKnee(t *testing.T) {
	const slots = 8
	const work = 20 * time.Millisecond
	sem := make(chan struct{}, slots)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sem <- struct{}{}
		time.Sleep(work)
		<-sem
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:     srv.URL,
		Method:  "GET",
		Headers: map[string]string{},
		Timeout: 5 * time.Second,
	}
	r := New(cfg)
	defer r.Close()

	// Short timings keep the test quick; levels 2,4,8,16,32,64.
	res := r.RunCapacityProbe(context.Background(), CapacityConfig{
		Start:    2,
		Factor:   2,
		Max:      64,
		StepRamp: 500 * time.Millisecond,
		StepHold: 1500 * time.Millisecond,
	})

	if len(res.Steps) < 3 {
		t.Fatalf("expected several probed steps, got %d", len(res.Steps))
	}
	t.Logf("reason=%s knee=%d maxRPS=%.0f baseLat=%.1fms satLat=%.1fms",
		res.Reason, res.KneeConcurrency, res.MaxRPS, res.BaselineLatencyMs, res.SaturationLatencyMs)
	for _, s := range res.Steps {
		t.Logf("  c=%-3d rps=%-7.0f avgLat=%-7.1fms err=%.1f%%", s.Concurrency, s.RPS, s.AvgLatencyMs, s.ErrorRate*100)
	}

	ceiling := float64(slots) / work.Seconds() // ~400 rps

	// It should have saturated (not "max_reached") somewhere sensible.
	if res.Reason == "max_reached" {
		t.Fatalf("expected the probe to detect saturation below max, got max_reached")
	}
	// MaxRPS should be in the neighbourhood of the ceiling (not wildly off).
	if res.MaxRPS < ceiling*0.5 || res.MaxRPS > ceiling*1.8 {
		t.Fatalf("MaxRPS %.0f not near the ~%.0f ceiling", res.MaxRPS, ceiling)
	}
	// Knee should be around the number of slots (within a few doublings).
	if res.KneeConcurrency < 4 || res.KneeConcurrency > 32 {
		t.Fatalf("knee concurrency %d not near the ~%d slot ceiling", res.KneeConcurrency, slots)
	}
	// Saturation latency should exceed baseline (queuing kicked in).
	if res.SaturationLatencyMs <= res.BaselineLatencyMs {
		t.Fatalf("expected saturation latency (%.1f) > baseline (%.1f)", res.SaturationLatencyMs, res.BaselineLatencyMs)
	}
}
