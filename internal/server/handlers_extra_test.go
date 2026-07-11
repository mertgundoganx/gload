package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mertgundoganx/gload/internal/storage"
)

// seedServiceWithResult inserts a service plus one completed test result directly
// through the store, so read endpoints have data without running a load test.
func seedServiceWithResult(t *testing.T, srv *Server, name string) int64 {
	t.Helper()
	svc := &storage.Service{
		Name: name, URL: "http://example.com", Method: "GET",
		Concurrency: 5, Duration: "2s", Timeout: "30s",
	}
	if err := srv.store.CreateService(svc); err != nil {
		t.Fatal(err)
	}
	r := &storage.TestResult{
		DurationMs: 2000, TotalReqs: 100, Errors: 2, RPS: 50,
		AvgLatencyMs: 12, P50LatencyMs: 10, P95LatencyMs: 25, P99LatencyMs: 40,
		MinLatencyMs: 1, MaxLatencyMs: 80, StatusCodes: map[int]int{200: 98, 500: 2},
		Status: "pass", AssertionResults: "[]", RunConfig: "{}",
	}
	if err := srv.store.SaveTestResult(svc.ID, r); err != nil {
		t.Fatal(err)
	}
	return svc.ID
}

// rawGet returns the status code and body for endpoints that don't return JSON
// (HTML reports, CSV/JSON exports, XML).
func rawGet(t *testing.T, ts string, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(ts + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(b)
}

func TestResultReadEndpoints(t *testing.T) {
	srv, ts := setupTestServer(t)
	id := seedServiceWithResult(t, srv, "read-svc")
	base := fmt.Sprintf("/api/services/%d", id)

	// JSON endpoints
	if resp, body := apiCall(t, ts, "GET", base+"/results", nil); resp.StatusCode != 200 || body["total_reqs"] == nil {
		t.Errorf("/results: status %d body %v", resp.StatusCode, body)
	}
	if resp, arr := apiCallArray(t, ts, "GET", base+"/history"); resp.StatusCode != 200 || len(arr) != 1 {
		t.Errorf("/history: status %d len %d", resp.StatusCode, len(arr))
	}
	if resp, _ := apiCall(t, ts, "GET", base+"/insights", nil); resp.StatusCode != 200 {
		t.Errorf("/insights: status %d", resp.StatusCode)
	}
	if resp, _ := apiCall(t, ts, "GET", base+"/capacity", nil); resp.StatusCode != 200 {
		t.Errorf("/capacity: status %d", resp.StatusCode)
	}

	// HTML / CSV / XML endpoints
	for _, tc := range []struct {
		path, contains string
	}{
		{base + "/report", "<html"},
		{base + "/pdf", "window.print"},
		{base + "/junit", "<testsuite"},
		{base + "/results/export?format=csv", "total_reqs"},
		{base + "/results/export?format=json", "\"rps\""},
	} {
		code, body := rawGet(t, ts.URL, tc.path)
		if code != 200 {
			t.Errorf("%s: status %d", tc.path, code)
		}
		if !strings.Contains(body, tc.contains) {
			t.Errorf("%s: body missing %q", tc.path, tc.contains)
		}
	}
}

func TestResultNoteAndShare(t *testing.T) {
	srv, ts := setupTestServer(t)
	id := seedServiceWithResult(t, srv, "note-svc")
	results, _ := srv.store.ListResults(id, 1)
	rid := results[0].ID
	base := fmt.Sprintf("/api/services/%d", id)

	// Update note.
	if resp, _ := apiCall(t, ts, "PUT", fmt.Sprintf("%s/results/%d/note", base, rid), map[string]interface{}{"note": "regression check"}); resp.StatusCode != 200 {
		t.Errorf("note update: status %d", resp.StatusCode)
	}
	got, _ := srv.store.GetResult(rid)
	if got.Note != "regression check" {
		t.Errorf("note not persisted: %q", got.Note)
	}

	// Share page is HTML.
	if code, body := rawGet(t, ts.URL, fmt.Sprintf("%s/results/%d/share", base, rid)); code != 200 || !strings.Contains(body, "<html") {
		t.Errorf("share: status %d", code)
	}

	// Delete the result.
	if resp, _ := apiCall(t, ts, "DELETE", fmt.Sprintf("%s/results/%d", base, rid), nil); resp.StatusCode != 200 {
		t.Errorf("delete result: status %d", resp.StatusCode)
	}
	if left, _ := srv.store.ListResults(id, 10); len(left) != 0 {
		t.Errorf("result not deleted: %d remain", len(left))
	}
}

func TestCapacityEndpoints(t *testing.T) {
	srv, ts := setupTestServer(t)
	svc := &storage.Service{Name: "cap-svc", URL: "http://x", Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s"}
	srv.store.CreateService(svc)
	srv.store.SaveCapacityResult(svc.ID, `{"steps":[{"concurrency":5,"rps":100,"avg_latency_ms":10,"error_rate":0}],"knee_concurrency":5,"max_rps":100,"baseline_latency_ms":10,"saturation_latency_ms":20,"reason":"throughput_plateau"}`)
	base := fmt.Sprintf("/api/services/%d", svc.ID)

	// History list.
	resp, arr := apiCallArray(t, ts, "GET", base+"/capacity-probe")
	if resp.StatusCode != 200 || len(arr) != 1 {
		t.Fatalf("capacity-probe history: status %d len %d", resp.StatusCode, len(arr))
	}
	// Report (HTML).
	if code, body := rawGet(t, ts.URL, base+"/capacity-report"); code != 200 || !strings.Contains(body, "<html") {
		t.Errorf("capacity-report: status %d", code)
	}
	// Delete the run.
	runs, _ := srv.store.ListCapacityRuns(svc.ID)
	if resp, _ := apiCall(t, ts, "DELETE", fmt.Sprintf("%s/capacity-probe/%d", base, runs[0].ID), nil); resp.StatusCode != 200 {
		t.Errorf("delete capacity run: status %d", resp.StatusCode)
	}
}

func TestNotFoundAndBadInput(t *testing.T) {
	_, ts := setupTestServer(t)

	// Unknown service id → 404 on read endpoints.
	for _, p := range []string{"/api/services/9999", "/api/services/9999/results", "/api/services/9999/history"} {
		if code, _ := rawGet(t, ts.URL, p); code != 404 {
			t.Errorf("%s: expected 404, got %d", p, code)
		}
	}
	// Non-numeric id → 400.
	if code, _ := rawGet(t, ts.URL, "/api/services/abc"); code != 400 {
		t.Errorf("invalid id: expected 400, got %d", code)
	}
	// Results with no data yet → 404.
	_, ts2 := setupTestServer(t)
	srv2Resp, _ := apiCall(t, ts2, "POST", "/api/services", map[string]interface{}{"name": "empty", "url": "http://x", "method": "GET"})
	_ = srv2Resp
	if code, _ := rawGet(t, ts2.URL, "/api/services/1/results"); code != 404 {
		t.Errorf("no-results: expected 404, got %d", code)
	}
}

func TestRunPatternValidation(t *testing.T) {
	srv, ts := setupTestServer(t)
	svc := &storage.Service{Name: "pat", URL: "http://x", Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s"}
	srv.store.CreateService(svc)
	base := fmt.Sprintf("/api/services/%d", svc.ID)

	// Unknown pattern name → 400.
	if resp, _ := apiCall(t, ts, "POST", base+"/run-pattern", map[string]interface{}{"pattern_name": "Nope"}); resp.StatusCode != 400 {
		t.Errorf("unknown pattern: expected 400, got %d", resp.StatusCode)
	}
	// Neither name nor stages → 400.
	if resp, _ := apiCall(t, ts, "POST", base+"/run-pattern", map[string]interface{}{}); resp.StatusCode != 400 {
		t.Errorf("empty pattern body: expected 400, got %d", resp.StatusCode)
	}
}

func TestExportImportServices(t *testing.T) {
	srv, ts := setupTestServer(t)
	seedServiceWithResult(t, srv, "exp1")
	seedServiceWithResult(t, srv, "exp2")

	code, body := rawGet(t, ts.URL, "/api/services/export")
	if code != 200 || !strings.Contains(body, "exp1") || !strings.Contains(body, "exp2") {
		t.Fatalf("export: status %d, body %s", code, body[:min(len(body), 120)])
	}

	// Import into a fresh server.
	_, ts2 := setupTestServer(t)
	resp, _ := apiCall(t, ts2, "POST", "/api/services/import", []map[string]interface{}{
		{"name": "imported", "url": "http://imported.com", "method": "GET"},
	})
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Errorf("import: status %d", resp.StatusCode)
	}
	_, arr := apiCallArray(t, ts2, "GET", "/api/services")
	if len(arr) != 1 {
		t.Errorf("after import, %d services", len(arr))
	}
}

func TestQueueEndpoints(t *testing.T) {
	srv, ts := setupTestServer(t)
	svc := &storage.Service{Name: "q", URL: "http://x", Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s"}
	srv.store.CreateService(svc)

	// Add to queue.
	if resp, _ := apiCall(t, ts, "POST", "/api/queue", map[string]interface{}{"service_id": svc.ID}); resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Errorf("queue add: status %d", resp.StatusCode)
	}
	// List queue.
	if resp, _ := apiCall(t, ts, "GET", "/api/queue", nil); resp.StatusCode != 200 {
		t.Errorf("queue list: status %d", resp.StatusCode)
	}
}
