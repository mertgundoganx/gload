package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/prom"
	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/internal/storage"
	"github.com/mertgundoganx/gload/web"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// AppVersion is set from main to expose via the /api/version endpoint.
var AppVersion = "1.0.2"

type runState struct {
	runner   *runner.Runner
	cancel   context.CancelFunc
	duration time.Duration
	kind     string // "" for a normal test, "capacity" for a capacity probe
}

// Server holds all state for the web UI backend.
type Server struct {
	mu           sync.RWMutex
	store        *storage.Storage
	runs         map[int64]*runState
	queueMu      sync.Mutex // guards queueRunning
	queueRunning bool
	startTime    time.Time
	broadcast    *broadcastHub
}

// broadcastHub manages WebSocket connections for real-time event broadcasting.
type broadcastHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func newBroadcastHub() *broadcastHub {
	return &broadcastHub{clients: make(map[*websocket.Conn]bool)}
}

func (h *broadcastHub) add(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
}

func (h *broadcastHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *broadcastHub) send(event string, data interface{}) {
	msg := map[string]interface{}{"event": event, "data": data}

	h.mu.RLock()
	var failed []*websocket.Conn
	for conn := range h.clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := wsjson.Write(ctx, conn, msg); err != nil {
			failed = append(failed, conn)
		}
		cancel()
	}
	h.mu.RUnlock()

	for _, c := range failed {
		h.remove(c)
		c.Close(websocket.StatusGoingAway, "")
	}
}

// wsAcceptOptions returns AcceptOptions for WebSocket upgrades. By default the
// underlying library enforces a same-origin check (Origin host must match the
// Host header), preventing cross-site WebSocket hijacking. Additional allowed
// origins can be supplied via the GLOAD_WS_ORIGINS env var (comma-separated
// host patterns), and setting it to "*" disables the check entirely.
func wsAcceptOptions() *websocket.AcceptOptions {
	origins := os.Getenv("GLOAD_WS_ORIGINS")
	if origins == "" {
		return &websocket.AcceptOptions{}
	}
	if strings.TrimSpace(origins) == "*" {
		return &websocket.AcceptOptions{InsecureSkipVerify: true}
	}
	var patterns []string
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			patterns = append(patterns, o)
		}
	}
	return &websocket.AcceptOptions{OriginPatterns: patterns}
}

// New creates a new Server backed by the given storage.
func New(store *storage.Storage) *Server {
	return &Server{
		store:     store,
		runs:      make(map[int64]*runState),
		startTime: time.Now(),
		broadcast: newBroadcastHub(),
	}
}

// CreateHTTPServer creates and returns an *http.Server with all routes registered.
func (s *Server) CreateHTTPServer(port int) (*http.Server, error) {
	mux := http.NewServeMux()

	// Static files from embedded FS
	staticFS, err := fs.Sub(web.Assets, "static")
	if err != nil {
		return nil, fmt.Errorf("embedded static fs: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Pages
	mux.HandleFunc("/", s.handleIndex)

	// API
	mux.HandleFunc("/api/templates", s.handleTemplates)
	mux.HandleFunc("/api/ws/events", s.handleBroadcast)
	mux.HandleFunc("/api/patterns", s.handlePatterns)
	mux.HandleFunc("/api/settings/test-notification", s.handleTestNotification)
	mux.HandleFunc("/api/settings/purge-now", s.handlePurgeNow)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/workers/test", s.handleTestWorkers)
	mux.HandleFunc("/api/queue/bulk-add", s.handleBulkQueueAdd)
	mux.HandleFunc("/api/queue", s.handleQueue)
	mux.HandleFunc("/api/queue/", s.handleQueueRoute)
	mux.HandleFunc("/api/services/bulk-delete", s.handleBulkDelete)
	mux.HandleFunc("/api/services/export", s.handleExport)
	mux.HandleFunc("/api/services/import", s.handleImport)
	mux.HandleFunc("/api/services", s.handleServices)
	mux.HandleFunc("/api/services/", s.handleServiceRoute)
	mux.HandleFunc("/api/schedules", s.handleSchedules)
	mux.HandleFunc("/api/schedules/", s.handleScheduleRoute)
	mux.HandleFunc("/api/workers", s.handleWorkers)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/workspaces", s.handleWorkspaces)
	mux.HandleFunc("/api/workspaces/", s.handleWorkspaceRoute)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/compare-report", s.handleCompareReport)
	mux.HandleFunc("/metrics", prom.Handler())
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)

	// pprof profiling endpoints — disabled by default; opt in with
	// GLOAD_PPROF=1 since they expose internal state and allow expensive
	// CPU/heap profiling to be triggered by anyone who can reach the server.
	if os.Getenv("GLOAD_PPROF") == "1" {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		logger.Info("pprof profiling endpoints enabled")
	}

	addr := fmt.Sprintf(":%d", port)
	logger.Info("web server listening", logger.Fields("addr", addr))
	handler := securityHeaders(limitBody(gzipMiddleware(mux)))
	return &http.Server{Addr: addr, Handler: handler}, nil
}

// ListenAndServe starts the HTTP server on the given port.
func (s *Server) ListenAndServe(port int) error {
	httpServer, err := s.CreateHTTPServer(port)
	if err != nil {
		return err
	}
	return httpServer.ListenAndServe()
}

// WaitForTests waits for all running tests to complete or the context to expire.
func (s *Server) WaitForTests(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Warn("timeout waiting for tests, forcing shutdown")
			// Cancel all running tests
			s.mu.Lock()
			for _, rs := range s.runs {
				rs.cancel()
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.mu.RLock()
			count := len(s.runs)
			s.mu.RUnlock()
			if count == 0 {
				logger.Info("all tests completed, shutting down")
				return
			}
			logger.Info("waiting for running tests to complete", logger.Fields("count", count))
		}
	}
}

// ---------- handlers ----------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := web.Assets.ReadFile("templates/index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>gload</title></head><body><h1>gload web UI</h1><p>Place your template at web/templates/index.html</p></body></html>`)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ---------- Broadcast WebSocket ----------

func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, wsAcceptOptions())
	if err != nil {
		return
	}

	s.broadcast.add(conn)
	defer func() {
		s.broadcast.remove(conn)
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	// Keep connection alive — read messages (we don't expect any, but need to consume)
	ctx := conn.CloseRead(r.Context())
	<-ctx.Done()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ---------- retention worker ----------

// StartRetentionWorker starts a background goroutine that periodically purges
// old test results based on the "retention_days" setting.
func (s *Server) StartRetentionWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			retentionDays, _ := s.store.GetSetting("retention_days")
			days := 0
			if retentionDays != "" {
				fmt.Sscanf(retentionDays, "%d", &days)
			}
			if days > 0 {
				cutoff := time.Now().AddDate(0, 0, -days)
				deleted, err := s.store.PurgeOldResults(cutoff)
				if err != nil {
					logger.Error("retention purge failed", logger.Fields("error", err.Error()))
				} else if deleted > 0 {
					logger.Info("retention purge complete", logger.Fields("deleted", deleted, "days", days))
				}
			}
		}
	}()
}
