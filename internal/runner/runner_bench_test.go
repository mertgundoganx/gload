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

// BenchmarkExecuteStatic measures the full per-request path against a local
// no-op server for a static request (no templating). Captures the templating
// short-circuit and sampled tracing, plus the sharded metrics recording.
func BenchmarkExecuteStatic(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{"Accept": "application/json"},
		Concurrency: 8,
		Timeout:     5 * time.Second,
		HTTP2:       false,
	}
	r := New(cfg)
	defer r.Close()
	ctx := context.Background()

	var next int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		wc := r.newWorkerCtx(int(atomic.AddInt64(&next, 1)))
		for pb.Next() {
			r.executeSingleRequest(ctx, r.client, "GET", cfg.URL, "", cfg.Headers, nil, wc)
		}
	})
}
