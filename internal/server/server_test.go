package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/internal/storage"

	"nhooyr.io/websocket"
)

func setupTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	srv := New(store)
	httpSrv, err := srv.CreateHTTPServer(0)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(httpSrv.Handler)
	t.Cleanup(ts.Close)

	return srv, ts
}

func apiCall(t *testing.T, ts *httptest.Server, method, path string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return resp, result
}

func apiCallArray(t *testing.T, ts *httptest.Server, method, path string) (*http.Response, []interface{}) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var result []interface{}
	json.Unmarshal(respBody, &result)
	return resp, result
}

// ---- Health & Version ----

func TestHealthEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, body := apiCall(t, ts, "GET", "/health", nil)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "healthy" {
		t.Errorf("status should be healthy, got %v", body["status"])
	}
	if body["uptime"] == nil {
		t.Error("missing uptime")
	}
	if body["version"] == nil {
		t.Error("missing version")
	}
	if body["database"] == nil {
		t.Error("missing database info")
	}
	if body["memory"] == nil {
		t.Error("missing memory info")
	}
}

func TestReadyEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, body := apiCall(t, ts, "GET", "/ready", nil)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ready" {
		t.Error("status should be ready")
	}
}

func TestVersionEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, body := apiCall(t, ts, "GET", "/api/version", nil)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["go"] == nil {
		t.Error("missing go version")
	}
}

// ---- Service CRUD ----

func TestCreateService(t *testing.T) {
	_, ts := setupTestServer(t)

	svc := map[string]interface{}{
		"name": "Test API", "url": "https://httpbin.org/get",
		"method": "GET", "concurrency": 5, "duration": "10s",
	}
	resp, body := apiCall(t, ts, "POST", "/api/services", svc)
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	if body["name"] != "Test API" {
		t.Error("wrong name")
	}
	if body["id"] == nil {
		t.Error("missing id")
	}
}

func TestListServices(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create 2 services
	apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Svc1", "url": "https://example.com", "method": "GET"})
	apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Svc2", "url": "https://example.com", "method": "POST"})

	resp, arr := apiCallArray(t, ts, "GET", "/api/services")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 services, got %d", len(arr))
	}
}

func TestUpdateService(t *testing.T) {
	_, ts := setupTestServer(t)

	_, created := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Original", "url": "https://example.com", "method": "GET"})
	id := fmt.Sprintf("%.0f", created["id"].(float64))

	resp, updated := apiCall(t, ts, "PUT", "/api/services/"+id, map[string]interface{}{
		"name": "Updated", "url": "https://updated.com", "method": "POST",
	})
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if updated["name"] != "Updated" {
		t.Error("name not updated")
	}
}

func TestDeleteService(t *testing.T) {
	_, ts := setupTestServer(t)

	_, created := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "ToDelete", "url": "https://example.com", "method": "GET"})
	id := fmt.Sprintf("%.0f", created["id"].(float64))

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/services/"+id, nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 204 {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	resp2, _ := apiCall(t, ts, "GET", "/api/services/"+id, nil)
	if resp2.StatusCode != 404 {
		t.Error("should be 404 after delete")
	}
}

func TestCloneService(t *testing.T) {
	_, ts := setupTestServer(t)

	_, created := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Original", "url": "https://example.com", "method": "GET"})
	id := fmt.Sprintf("%.0f", created["id"].(float64))

	resp, cloned := apiCall(t, ts, "POST", "/api/services/"+id+"/clone", nil)
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	name, _ := cloned["name"].(string)
	if name == "" || name == "Original" {
		t.Error("clone should have modified name")
	}
}

// ---- Settings ----

func TestSettings(t *testing.T) {
	srv, ts := setupTestServer(t)

	// Set
	resp, _ := apiCall(t, ts, "PUT", "/api/settings", map[string]interface{}{
		"theme": "dark", "webhook_url": "https://example.com/hook",
	})
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Get
	resp2, body := apiCall(t, ts, "GET", "/api/settings", nil)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}
	if body["theme"] != "dark" {
		t.Error("theme not saved")
	}
	// Sensitive settings must be masked on read, never echoed in cleartext.
	if body["webhook_url"] == "https://example.com/hook" {
		t.Error("webhook_url should be masked on GET, not returned in cleartext")
	}
	if body["webhook_url"] != secretMask {
		t.Errorf("expected masked webhook_url, got %q", body["webhook_url"])
	}

	// Re-saving the mask must not overwrite the stored secret.
	apiCall(t, ts, "PUT", "/api/settings", map[string]interface{}{
		"webhook_url": secretMask,
	})
	if got, _ := srv.store.GetSetting("webhook_url"); got != "https://example.com/hook" {
		t.Errorf("mask re-save clobbered secret, stored=%q", got)
	}
}

// ---- Workspaces ----

func TestWorkspaceCRUD(t *testing.T) {
	_, ts := setupTestServer(t)

	// List (should have Default)
	resp, arr := apiCallArray(t, ts, "GET", "/api/workspaces")
	if resp.StatusCode != 200 {
		t.Error("list failed")
	}
	if len(arr) < 1 {
		t.Error("should have at least Default workspace")
	}

	// Create
	resp2, ws := apiCall(t, ts, "POST", "/api/workspaces", map[string]interface{}{
		"name": "Team A", "slug": "team-a", "description": "Test team",
	})
	if resp2.StatusCode != 201 {
		t.Errorf("create failed: %d", resp2.StatusCode)
	}
	if ws["slug"] != "team-a" {
		t.Error("wrong slug")
	}
}

// ---- Schedules ----

func TestScheduleCRUD(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create service first
	_, svc := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Sched Test", "url": "https://example.com", "method": "GET"})
	svcID := fmt.Sprintf("%.0f", svc["id"].(float64))

	// Create schedule
	resp, sched := apiCall(t, ts, "POST", "/api/schedules", map[string]interface{}{
		"service_id": svc["id"], "cron_expr": "0 3 * * *",
	})
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	if sched["cron_expr"] != "0 3 * * *" {
		t.Error("wrong cron")
	}
	_ = svcID // used for context

	// List
	resp2, arr := apiCallArray(t, ts, "GET", "/api/schedules")
	if resp2.StatusCode != 200 {
		t.Error("list failed")
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 schedule, got %d", len(arr))
	}
}

// ---- Patterns & Templates ----

func TestPatternsEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, arr := apiCallArray(t, ts, "GET", "/api/patterns")
	if resp.StatusCode != 200 {
		t.Error("expected 200")
	}
	if len(arr) < 4 {
		t.Errorf("expected at least 4 patterns, got %d", len(arr))
	}
}

func TestTemplatesEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, arr := apiCallArray(t, ts, "GET", "/api/templates")
	if resp.StatusCode != 200 {
		t.Error("expected 200")
	}
	if len(arr) < 5 {
		t.Errorf("expected at least 5 templates, got %d", len(arr))
	}
}

// ---- Plugins ----

func TestPluginsEndpoint(t *testing.T) {
	_, ts := setupTestServer(t)
	resp, body := apiCall(t, ts, "GET", "/api/plugins", nil)
	if resp.StatusCode != 200 {
		t.Error("expected 200")
	}
	if body["protocols"] == nil {
		t.Error("missing protocols")
	}
}

// ---- Metrics ----

func TestPrometheusMetrics(t *testing.T) {
	_, ts := setupTestServer(t)
	req, _ := http.NewRequest("GET", ts.URL+"/metrics", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(body) == 0 {
		t.Error("empty metrics")
	}
}

// ---- Bulk Operations ----

func TestBulkDelete(t *testing.T) {
	_, ts := setupTestServer(t)

	_, s1 := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Bulk1", "url": "https://a.com", "method": "GET"})
	_, s2 := apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Bulk2", "url": "https://b.com", "method": "GET"})

	ids := []float64{s1["id"].(float64), s2["id"].(float64)}
	resp, body := apiCall(t, ts, "POST", "/api/services/bulk-delete", map[string]interface{}{"ids": ids})
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	deleted, _ := body["deleted"].(float64)
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %.0f", deleted)
	}

	// Verify empty
	_, arr := apiCallArray(t, ts, "GET", "/api/services")
	if len(arr) != 0 {
		t.Errorf("expected 0 services, got %d", len(arr))
	}
}

// ---- WebSocket origin enforcement ----

func TestWebSocketOrigin(t *testing.T) {
	_, ts := setupTestServer(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// Same-origin (Origin host matches the server host) must be accepted.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	host := strings.TrimPrefix(ts.URL, "http://")
	conn, _, err := websocket.Dial(ctx, wsURL+"/api/ws/events", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"http://" + host}},
	})
	if err != nil {
		t.Fatalf("same-origin websocket should connect, got: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")

	// Cross-origin must be rejected by the same-origin check.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	conn2, _, err := websocket.Dial(ctx2, wsURL+"/api/ws/events", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"http://evil.example.com"}},
	})
	if err == nil {
		conn2.Close(websocket.StatusNormalClosure, "")
		t.Fatal("cross-origin websocket should be rejected")
	}
}

// ---- Export/Import ----

func TestExportImport(t *testing.T) {
	_, ts := setupTestServer(t)

	apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Export1", "url": "https://a.com", "method": "GET"})
	apiCall(t, ts, "POST", "/api/services", map[string]interface{}{"name": "Export2", "url": "https://b.com", "method": "POST"})

	// Export
	resp, arr := apiCallArray(t, ts, "GET", "/api/services/export")
	if resp.StatusCode != 200 {
		t.Error("export failed")
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 exported, got %d", len(arr))
	}
}

// TestImportWorkspaceRemap ensures services imported with a workspace_id that
// doesn't exist on this instance (e.g. exported from another instance) are
// remapped to the default workspace instead of becoming orphaned/invisible.
func TestImportWorkspaceRemap(t *testing.T) {
	_, ts := setupTestServer(t)

	// Import a service pointing at a non-existent workspace.
	resp, _ := apiCall(t, ts, "POST", "/api/services/import", []map[string]interface{}{
		{"name": "orphan", "url": "https://a.com", "workspace_id": 9999},
		{"name": "bad", "url": "not-a-url"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("import failed: %d", resp.StatusCode)
	}

	// The orphan must be visible under the default workspace; the invalid one skipped.
	_, list := apiCallArray(t, ts, "GET", "/api/services?workspace=default")
	found := false
	for _, item := range list {
		s, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if s["name"] == "orphan" {
			found = true
			if wsID, _ := s["workspace_id"].(float64); wsID == 9999 || wsID == 0 {
				t.Errorf("orphan not remapped, workspace_id=%v", s["workspace_id"])
			}
		}
		if s["name"] == "bad" {
			t.Error("invalid service should have been skipped")
		}
	}
	if !found {
		t.Error("imported service is invisible under default workspace (orphaned)")
	}
}
