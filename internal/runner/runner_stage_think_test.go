package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

// TestStagesHonorThinkTime verifies that a staged (pattern) run applies think
// time between iterations, so each virtual user models a real user who pauses
// between actions rather than firing flat-out. With ~10 held VUs and a 200-400ms
// think time over ~2s, the request count must be far below what an un-throttled
// flat-out run would produce against an instant endpoint.
func TestStagesHonorThinkTime(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:          srv.URL,
		Method:       "GET",
		Headers:      map[string]string{},
		Timeout:      5 * time.Second,
		ThinkTime:    200 * time.Millisecond,
		ThinkTimeMax: 400 * time.Millisecond,
		Stages: []config.Stage{
			{Duration: 500 * time.Millisecond, Target: 10}, // ramp to 10
			{Duration: 2 * time.Second, Target: 10},        // hold 10
		},
	}

	r := New(cfg)
	defer r.Close()
	r.Run(context.Background())

	total := r.Metrics.Snapshot().TotalReqs
	if total == 0 {
		t.Fatal("expected some requests during staged run")
	}
	// 10 VUs holding ~2s with a ~300ms mean think time do on the order of
	// 10 * (2.5s / ~0.3s) ≈ ~80 requests. A flat-out run against an instant
	// local endpoint would be in the tens of thousands. Assert an upper bound
	// that only holds if think time is actually being applied.
	if total > 400 {
		t.Fatalf("think time not applied in staged run: got %d requests (expected well under 400)", total)
	}
	t.Logf("staged think-time ok: totalReqs=%d serverHits=%d", total, atomic.LoadInt64(&hits))
}
