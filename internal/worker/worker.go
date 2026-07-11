package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/pkg/config"
)

// WorkerServer handles incoming test requests from a coordinator.
type WorkerServer struct {
	mu      sync.Mutex
	running *runner.Runner
	cancel  context.CancelFunc
}

// NewWorkerServer creates a new WorkerServer.
func NewWorkerServer() *WorkerServer {
	return &WorkerServer{}
}

// ListenAndServe starts the worker HTTP server on the given port.
func (ws *WorkerServer) ListenAndServe(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/worker/run", ws.handleRun)
	mux.HandleFunc("/worker/status", ws.handleStatus)
	mux.HandleFunc("/worker/stop", ws.handleStop)
	mux.HandleFunc("/worker/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("gload worker listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (ws *WorkerServer) handleRun(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ws.mu.Lock()
	if ws.running != nil {
		ws.mu.Unlock()
		http.Error(w, "already running", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := runner.New(&cfg)
	ws.running = run
	ws.cancel = cancel
	ws.mu.Unlock()

	go func() {
		run.Run(ctx)
		ws.mu.Lock()
		ws.running = nil
		ws.cancel = nil
		ws.mu.Unlock()
	}()

	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (ws *WorkerServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	ws.mu.Lock()
	run := ws.running
	ws.mu.Unlock()

	if run == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "idle"})
		return
	}

	snap := run.Metrics.Snapshot()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "running",
		"total_reqs":     snap.TotalReqs,
		"rps":            snap.RPS,
		"errors":         snap.Errors,
		"avg_latency_ms": float64(snap.AvgLatency.Microseconds()) / 1000,
	})
}

func (ws *WorkerServer) handleStop(w http.ResponseWriter, r *http.Request) {
	ws.mu.Lock()
	if ws.cancel != nil {
		ws.cancel()
	}
	ws.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}
