package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/internal/storage"
)

func TestWorkersAndPurge(t *testing.T) {
	srv, ts := setupTestServer(t)

	// Configure worker URLs, then list + test them.
	apiCall(t, ts, "PUT", "/api/settings", map[string]interface{}{"worker_urls": "http://127.0.0.1:59999"})
	if resp, _ := apiCall(t, ts, "GET", "/api/workers", nil); resp.StatusCode != 200 {
		t.Errorf("GET /api/workers: %d", resp.StatusCode)
	}
	if resp, _ := apiCall(t, ts, "POST", "/api/workers/test", map[string]interface{}{"urls": "http://127.0.0.1:59999"}); resp.StatusCode != 200 {
		t.Errorf("POST /api/workers/test: %d", resp.StatusCode)
	}

	// Retention purge.
	apiCall(t, ts, "PUT", "/api/settings", map[string]interface{}{"retention_days": "30"})
	seedServiceWithResult(t, srv, "purge-svc")
	if resp, _ := apiCall(t, ts, "POST", "/api/settings/purge-now", nil); resp.StatusCode != 200 {
		t.Errorf("purge-now: %d", resp.StatusCode)
	}
}

func TestTestNotification(t *testing.T) {
	_, ts := setupTestServer(t)
	// With no webhook configured this should respond with a clear status (not 200 success),
	// but must not 500-panic.
	resp, _ := apiCall(t, ts, "POST", "/api/settings/test-notification", nil)
	if resp.StatusCode == 0 || resp.StatusCode >= 500 {
		t.Errorf("test-notification returned %d", resp.StatusCode)
	}
}

func TestCompareReport(t *testing.T) {
	srv, ts := setupTestServer(t)
	id1 := seedServiceWithResult(t, srv, "cmp1")
	id2 := seedServiceWithResult(t, srv, "cmp2")
	r1, _ := srv.store.ListResults(id1, 1)
	r2, _ := srv.store.ListResults(id2, 1)

	code, body := rawGet(t, ts.URL, fmt.Sprintf("/api/compare-report?ids=%d,%d", r1[0].ID, r2[0].ID))
	if code != 200 {
		t.Errorf("compare-report: status %d", code)
	}
	if len(body) == 0 {
		t.Error("compare-report body empty")
	}

	// Fewer than two ids → 400.
	if code, _ := rawGet(t, ts.URL, fmt.Sprintf("/api/compare-report?ids=%d", r1[0].ID)); code != 400 {
		t.Errorf("single-id compare: expected 400, got %d", code)
	}
}

func TestGitHubCommentNoEnv(t *testing.T) {
	srv, ts := setupTestServer(t)
	id := seedServiceWithResult(t, srv, "gh-svc")
	// No GitHub env configured → handler should report an error, not succeed.
	resp, _ := apiCall(t, ts, "POST", fmt.Sprintf("/api/services/%d/github-comment", id), nil)
	if resp.StatusCode == 200 {
		t.Errorf("github-comment should fail without env, got 200")
	}
}

func TestRunDistributedNoWorkers(t *testing.T) {
	srv, ts := setupTestServer(t)
	svc := &storage.Service{Name: "dist", URL: "http://x", Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s"}
	srv.store.CreateService(svc)
	// No workers configured → should error rather than start.
	resp, _ := apiCall(t, ts, "POST", fmt.Sprintf("/api/services/%d/run-distributed", svc.ID), nil)
	if resp.StatusCode == 200 {
		t.Errorf("run-distributed with no workers should not succeed")
	}
}

func TestRunStopLifecycle(t *testing.T) {
	srv, ts := setupTestServer(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	svc := &storage.Service{Name: "runnable", URL: target.URL, Method: "GET", Concurrency: 2, Duration: "3s", Timeout: "5s"}
	srv.store.CreateService(svc)
	base := fmt.Sprintf("/api/services/%d", svc.ID)

	if resp, body := apiCall(t, ts, "POST", base+"/run", nil); resp.StatusCode != 200 || body["status"] == nil {
		t.Fatalf("run: status %d body %v", resp.StatusCode, body)
	}

	// Give it a moment to register as running, then stop it.
	time.Sleep(200 * time.Millisecond)
	if resp, _ := apiCall(t, ts, "POST", base+"/stop", nil); resp.StatusCode != 200 {
		t.Errorf("stop: status %d", resp.StatusCode)
	}

	// Eventually the run clears and a result may be persisted.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.RLock()
		_, running := srv.runs[svc.ID]
		srv.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("run did not clear after stop")
}
