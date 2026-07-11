package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

func runToDone(cfg *config.Config) *Runner {
	r := New(cfg)
	go r.Run(context.Background())
	<-r.Done
	return r
}

func TestRunnerPOSTWithFaker(t *testing.T) {
	t.Parallel()
	// Computed inside the handler goroutine and read atomically to avoid a
	// data race between the server and test goroutines.
	var sawPlaceholder, sawBody atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			sawBody.Store(true)
		}
		if strings.Contains(string(b), "{{") {
			sawPlaceholder.Store(true)
		}
		w.WriteHeader(201)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL: srv.URL, Method: "POST", ContentType: "json",
		Body:        `{"id":"{{gen.uuid}}","n":"{{gen.name}}"}`,
		Headers:     map[string]string{"Content-Type": "application/json"},
		Concurrency: 1, Duration: 700 * time.Millisecond, Timeout: 5 * time.Second,
	}
	snap := runToDone(cfg).Metrics.Snapshot()
	if snap.StatusCodes[201] == 0 {
		t.Fatalf("expected 201s, got %v", snap.StatusCodes)
	}
	if !sawBody.Load() {
		t.Error("server never received a request body")
	}
	if sawPlaceholder.Load() {
		t.Error("faker placeholders were not substituted in the request body")
	}
}

func TestRunnerUserAgent(t *testing.T) {
	t.Parallel()
	var sawBranded, sawCustom atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		if strings.HasPrefix(ua, "gload/") {
			sawBranded.Store(true)
		}
		if ua == "CustomBot/9.9" {
			sawCustom.Store(true)
		}
	}))
	defer srv.Close()

	// Default: gload brands the request.
	base := &config.Config{URL: srv.URL, Method: "GET", Headers: map[string]string{},
		Concurrency: 1, Duration: 400 * time.Millisecond, Timeout: 5 * time.Second}
	runToDone(base)
	if !sawBranded.Load() {
		t.Error("expected a gload/* User-Agent by default")
	}

	// A user-supplied User-Agent takes precedence.
	custom := &config.Config{URL: srv.URL, Method: "GET",
		Headers:     map[string]string{"User-Agent": "CustomBot/9.9"},
		Concurrency: 1, Duration: 400 * time.Millisecond, Timeout: 5 * time.Second}
	runToDone(custom)
	if !sawCustom.Load() {
		t.Error("a user-set User-Agent should override the gload default")
	}
}

func TestRunnerValidationPass(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"SUCCESS"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL: srv.URL, Method: "GET", Headers: map[string]string{},
		Validations: []config.Validation{
			{Type: "contains", Value: "SUCCESS"},
			{Type: "status_code", Value: "200"},
			{Type: "json_path", Path: "status", Value: "SUCCESS"},
		},
		Concurrency: 1, Duration: 700 * time.Millisecond, Timeout: 5 * time.Second,
	}
	snap := runToDone(cfg).Metrics.Snapshot()
	if snap.TotalReqs == 0 {
		t.Fatal("no requests made")
	}
	if snap.ValidationFailures != 0 {
		t.Errorf("expected 0 validation failures, got %d", snap.ValidationFailures)
	}
}

func TestRunnerValidationFail(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "totally different body")
	}))
	defer srv.Close()

	cfg := &config.Config{
		URL: srv.URL, Method: "GET", Headers: map[string]string{},
		Validations: []config.Validation{{Type: "contains", Value: "SUCCESS"}},
		Concurrency: 1, Duration: 500 * time.Millisecond, Timeout: 5 * time.Second,
	}
	snap := runToDone(cfg).Metrics.Snapshot()
	if snap.ValidationFailures == 0 {
		t.Error("expected validation failures when body lacks the expected string")
	}
}

func TestRunnerScenarioChaining(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"token": "abc123"})
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer abc123" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		URL: srv.URL, Method: "GET", Headers: map[string]string{},
		Steps: []config.Step{
			{
				Name: "login", URL: srv.URL + "/login", Method: "GET",
				Extractors: []config.Extractor{{Name: "token", Source: "body", Path: "token"}},
			},
			{
				Name: "data", URL: srv.URL + "/data", Method: "GET",
				Headers: map[string]string{"Authorization": "Bearer {{token}}"},
			},
		},
		Concurrency: 1, Duration: 700 * time.Millisecond, Timeout: 5 * time.Second,
	}
	snap := runToDone(cfg).Metrics.Snapshot()
	if snap.StatusCodes[200] == 0 {
		t.Fatalf("expected chained 200s on /data, got %v", snap.StatusCodes)
	}
	if snap.StatusCodes[401] != 0 {
		t.Errorf("chaining failed — got %d unauthorized responses", snap.StatusCodes[401])
	}
}
