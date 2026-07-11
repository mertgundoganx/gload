package worker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/pkg/config"
)

func do(ws *WorkerServer, h http.HandlerFunc, method, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	h(rec, req)
	var out map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestWorkerStatusIdle(t *testing.T) {
	ws := NewWorkerServer()
	_, out := do(ws, ws.handleStatus, "GET", "")
	if out["status"] != "idle" {
		t.Errorf("status = %v, want idle", out["status"])
	}
}

func TestWorkerRunInvalidJSON(t *testing.T) {
	ws := NewWorkerServer()
	rec, _ := do(ws, ws.handleRun, "POST", "{not json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkerStopWhenIdle(t *testing.T) {
	ws := NewWorkerServer()
	rec, out := do(ws, ws.handleStop, "POST", "")
	if rec.Code != 200 || out["status"] != "stopped" {
		t.Errorf("stop when idle = %d %v", rec.Code, out)
	}
}

func TestWorkerRunLifecycle(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	cfg := config.Config{
		URL: target.URL, Method: "GET", Concurrency: 1,
		Duration: 3 * time.Second, Timeout: 5 * time.Second,
	}
	body, _ := json.Marshal(cfg)

	ws := NewWorkerServer()
	rec, out := do(ws, ws.handleRun, "POST", string(body))
	if rec.Code != 200 || out["status"] != "started" {
		t.Fatalf("run = %d %v", rec.Code, out)
	}

	// While running, status reports "running" and a second run conflicts.
	if _, st := do(ws, ws.handleStatus, "GET", ""); st["status"] != "running" {
		t.Errorf("status while running = %v", st["status"])
	}
	conflict, _ := do(ws, ws.handleRun, "POST", string(bytes.TrimSpace(body)))
	if conflict.Code != http.StatusConflict {
		t.Errorf("second run = %d, want 409", conflict.Code)
	}

	// Stop cancels the run; the background goroutine clears state shortly after.
	if _, out := do(ws, ws.handleStop, "POST", ""); out["status"] != "stopped" {
		t.Errorf("stop = %v", out)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, st := do(ws, ws.handleStatus, "GET", ""); st["status"] == "idle" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("worker did not return to idle after stop")
}
