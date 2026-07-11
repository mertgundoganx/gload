package runner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

func TestRunnerBasic(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 2,
		Duration:    1 * time.Second,
		Timeout:     5 * time.Second,
	}

	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done

	snap := r.Metrics.Snapshot()
	if snap.TotalReqs == 0 {
		t.Fatal("expected TotalReqs > 0")
	}
	if snap.Errors != 0 {
		t.Fatalf("expected 0 errors, got %d", snap.Errors)
	}
	if snap.StatusCodes[200] == 0 {
		t.Fatal("expected StatusCodes[200] > 0")
	}
}

func TestRunnerWithErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 1,
		Duration:    1 * time.Second,
		Timeout:     5 * time.Second,
	}

	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done

	snap := r.Metrics.Snapshot()
	if snap.Errors == 0 {
		t.Fatal("expected errors > 0")
	}
	if snap.StatusCodes[500] == 0 {
		t.Fatal("expected StatusCodes[500] > 0")
	}
}

func TestRunnerRateLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 5,
		Duration:    2 * time.Second,
		Timeout:     5 * time.Second,
		RPS:         10,
	}

	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done

	snap := r.Metrics.Snapshot()
	// With RPS=10 and duration=2s, expect roughly 20 requests (tolerance +-10)
	if snap.TotalReqs < 10 || snap.TotalReqs > 30 {
		t.Fatalf("expected ~20 requests with RPS=10 for 2s, got %d", snap.TotalReqs)
	}
}

func TestRunnerContextCancel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 2,
		Duration:    10 * time.Second, // long duration
		Timeout:     5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := New(cfg)
	go r.Run(ctx)

	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-r.Done:
		// good
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not stop after context cancel")
	}
}

func TestRunnerCookieJar(t *testing.T) {
	t.Parallel()

	var cookieReceived atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err == nil && c.Value == "abc123" {
			cookieReceived.Add(1)
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL,
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 1,
		Duration:    1 * time.Second,
		Timeout:     5 * time.Second,
		CookieJar:   true,
	}

	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done

	// After the first request, every subsequent request should have the cookie.
	if cookieReceived.Load() == 0 {
		t.Fatal("expected cookie to be sent back on subsequent requests")
	}
}

func TestRunnerDynamicData(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	seenIDs := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the last path segment as the ID
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-1]
		mu.Lock()
		seenIDs[id] = true
		mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprint(w, id)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL:         srv.URL + "/item/{{id}}",
		Method:      "GET",
		Headers:     map[string]string{},
		Concurrency: 1,
		Duration:    1 * time.Second,
		Timeout:     5 * time.Second,
		DataSource: []map[string]string{
			{"id": "1"},
			{"id": "2"},
			{"id": "3"},
		},
	}

	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done

	mu.Lock()
	defer mu.Unlock()

	if len(seenIDs) < 2 {
		t.Fatalf("expected at least 2 different IDs to be hit, got %d: %v", len(seenIDs), seenIDs)
	}
}
