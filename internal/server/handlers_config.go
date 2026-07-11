package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/notifier"
	"github.com/mertgundoganx/gload/internal/plugin"
	"github.com/mertgundoganx/gload/internal/scheduler"
	"github.com/mertgundoganx/gload/internal/storage"
	"github.com/mertgundoganx/gload/pkg/config"
)

// ---------- Settings ----------

// secretMask is returned in place of stored credentials on GET /api/settings.
const secretMask = "••••••••"

// sensitiveSettingKeys are never echoed back to clients in cleartext.
var sensitiveSettingKeys = []string{
	"smtp_password",
	"webhook_url",
	"slack_webhook_url",
	"teams_webhook_url",
	"discord_webhook_url",
}

func isSensitiveSetting(key string) bool {
	for _, k := range sensitiveSettingKeys {
		if k == key {
			return true
		}
	}
	return false
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.GetAllSettings()
		if err != nil {
			dbError(w, err)
			return
		}
		// Never send stored secrets back to the client. Replace non-empty
		// sensitive values with a sentinel so the UI can show "configured"
		// without leaking the actual credential.
		for _, k := range sensitiveSettingKeys {
			if settings[k] != "" {
				settings[k] = secretMask
			}
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var settings map[string]string
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range settings {
			// Ignore masked sensitive values echoed back unchanged by the UI
			// so we don't overwrite the real secret with the mask sentinel.
			if v == secretMask && isSensitiveSetting(k) {
				continue
			}
			if err := s.store.SetSetting(k, v); err != nil {
				dbError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTestNotification sends a sample notification through every configured
// channel using the saved settings, and reports per-channel success/failure so
// users can verify their setup without running a real test.
func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sample := notifier.TestResult{
		ServiceName: "gload test notification",
		ServiceURL:  "https://example.com/health",
		Status:      "pass",
		TotalReqs:   12345,
		RPS:         987.6,
		AvgLatency:  12.3,
		P95Latency:  45.6,
		P99Latency:  78.9,
		ErrorRate:   0.0,
		Duration:    10000,
	}

	get := func(k string) string { v, _ := s.store.GetSetting(k); return strings.TrimSpace(v) }
	results := map[string]string{}
	attempted := 0
	send := func(name, url string, fn func(string, notifier.TestResult) error) {
		if url == "" {
			return
		}
		attempted++
		if err := fn(url, sample); err != nil {
			results[name] = "error: " + err.Error()
		} else {
			results[name] = "sent"
		}
	}

	send("webhook", get("webhook_url"), notifier.SendWebhook)
	send("slack", get("slack_webhook_url"), notifier.SendSlack)
	send("teams", get("teams_webhook_url"), notifier.SendTeams)
	send("discord", get("discord_webhook_url"), notifier.SendDiscord)

	if host := get("smtp_host"); host != "" && get("email_to") != "" {
		attempted++
		recipients := strings.Split(get("email_to"), ",")
		for i := range recipients {
			recipients[i] = strings.TrimSpace(recipients[i])
		}
		cfg := notifier.EmailConfig{
			SMTPHost: host, SMTPPort: get("smtp_port"),
			Username: get("smtp_username"), Password: get("smtp_password"),
			From: get("email_from"), To: recipients,
		}
		if err := notifier.SendEmail(cfg, sample); err != nil {
			results["email"] = "error: " + err.Error()
		} else {
			results["email"] = "sent"
		}
	}

	if attempted == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"attempted": 0,
			"message":   "No notification channels are configured. Add a webhook or email and save first.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"attempted": attempted, "results": results})
}

// ---------- Schedules ----------

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSchedules(w, r)
	case http.MethodPost:
		s.createSchedule(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScheduleRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid schedule id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.updateSchedule(w, r, id)
	case http.MethodDelete:
		s.deleteScheduleByID(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listSchedules(w http.ResponseWriter, _ *http.Request) {
	scheds, err := s.store.ListSchedules()
	if err != nil {
		dbError(w, err)
		return
	}

	// Fetch all service names once to avoid an N+1 query per schedule.
	services, err := s.store.ListServices()
	if err != nil {
		dbError(w, err)
		return
	}
	names := make(map[int64]string, len(services))
	for _, svc := range services {
		names[svc.ID] = svc.Name
	}

	list := make([]scheduleWithService, 0, len(scheds))
	for _, sched := range scheds {
		item := scheduleWithService{Schedule: sched}
		if name, ok := names[sched.ServiceID]; ok {
			item.ServiceName = name
		} else {
			item.ServiceName = fmt.Sprintf("Service #%d", sched.ServiceID)
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceID int64  `json:"service_id"`
		CronExpr  string `json:"cron_expr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ServiceID <= 0 {
		http.Error(w, "service_id is required", http.StatusBadRequest)
		return
	}
	body.CronExpr = strings.TrimSpace(body.CronExpr)
	if body.CronExpr == "" {
		http.Error(w, "cron_expr is required", http.StatusBadRequest)
		return
	}
	if err := scheduler.ValidateCron(body.CronExpr); err != nil {
		http.Error(w, "invalid cron: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify service exists.
	svc, err := s.store.GetService(body.ServiceID)
	if err != nil || svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	next := scheduler.NextCronTime(body.CronExpr, time.Now())
	sched := &storage.Schedule{
		ServiceID: body.ServiceID,
		CronExpr:  body.CronExpr,
		Enabled:   true,
		NextRun:   &next,
	}
	if err := s.store.CreateSchedule(sched); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sched)
}

func (s *Server) updateSchedule(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Enabled  *bool   `json:"enabled"`
		CronExpr *string `json:"cron_expr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Enabled == nil && body.CronExpr == nil {
		http.Error(w, "enabled or cron_expr is required", http.StatusBadRequest)
		return
	}

	// Update the cron expression (validated) and recompute next_run.
	if body.CronExpr != nil {
		expr := strings.TrimSpace(*body.CronExpr)
		if err := scheduler.ValidateCron(expr); err != nil {
			http.Error(w, "invalid cron: "+err.Error(), http.StatusBadRequest)
			return
		}
		next := scheduler.NextCronTime(expr, time.Now())
		if err := s.store.UpdateScheduleCron(id, expr, &next); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}

	if body.Enabled != nil {
		if err := s.store.UpdateScheduleEnabled(id, *body.Enabled); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// When enabling, make sure next_run is set so the UI shows it.
		if *body.Enabled {
			if sched, _ := s.store.GetSchedule(id); sched != nil {
				next := scheduler.NextCronTime(sched.CronExpr, time.Now())
				s.store.SetScheduleNextRun(id, &next)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) deleteScheduleByID(w http.ResponseWriter, _ *http.Request, id int64) {
	if err := s.store.DeleteSchedule(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RunScheduledTest runs a load test for the given service (fire-and-forget).
// This is called by the scheduler when a cron schedule is due.
func (s *Server) RunScheduledTest(serviceID int64) {
	s.runQueuedTest(serviceID, "scheduled")
}

// ---------- Workers (Distributed Testing) ----------

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlsStr, _ := s.store.GetSetting("worker_urls")
	if urlsStr == "" {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	type workerStatus struct {
		URL    string `json:"url"`
		Status string `json:"status"`
	}

	var workers []workerStatus
	for _, u := range strings.Split(urlsStr, ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		ws := workerStatus{URL: u, Status: "unreachable"}
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(u + "/worker/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ws.Status = "healthy"
			}
		}
		workers = append(workers, ws)
	}

	writeJSON(w, http.StatusOK, workers)
}

// workerHealth pings a worker's /worker/health and returns "healthy" or
// "unreachable".
func workerHealth(u string) string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u + "/worker/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return "healthy"
		}
	}
	return "unreachable"
}

// handleTestWorkers checks connectivity to the worker URLs supplied in the body
// (what's currently typed in the form), without needing to save first.
func (s *Server) handleTestWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URLs string `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	type workerStatus struct {
		URL    string `json:"url"`
		Status string `json:"status"`
	}
	out := []workerStatus{}
	for _, u := range strings.Split(body.URLs, ",") {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, workerStatus{URL: u, Status: workerHealth(u)})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePurgeNow immediately purges test results older than the configured
// retention window (the same logic the hourly worker runs).
func (s *Server) handlePurgeNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	daysStr, _ := s.store.GetSetting("retention_days")
	days := 0
	fmt.Sscanf(daysStr, "%d", &days)
	if days <= 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"deleted": 0,
			"message": "Retention is off (keep forever). Set a retention period to enable purging.",
		})
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	deleted, err := s.store.PurgeOldResults(cutoff)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": deleted, "days": days})
}

func (s *Server) runDistributed(w http.ResponseWriter, r *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	urlsStr, _ := s.store.GetSetting("worker_urls")
	if urlsStr == "" {
		http.Error(w, "no workers configured", http.StatusBadRequest)
		return
	}

	var workerURLs []string
	for _, u := range strings.Split(urlsStr, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			workerURLs = append(workerURLs, u)
		}
	}
	if len(workerURLs) == 0 {
		http.Error(w, "no workers configured", http.StatusBadRequest)
		return
	}

	dur, err := time.ParseDuration(svc.Duration)
	if err != nil {
		http.Error(w, "invalid duration: "+err.Error(), http.StatusBadRequest)
		return
	}
	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	// Split concurrency evenly across workers.
	perWorker := svc.Concurrency / len(workerURLs)
	if perWorker < 1 {
		perWorker = 1
	}

	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Concurrency:          perWorker,
		Duration:             dur,
		Timeout:              timeout,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		WarmupConns:          svc.WarmupConns,
		AdaptiveConcurrency:  svc.AdaptiveConcurrency != 0,
		AdaptiveTargetMs:     svc.AdaptiveTargetMs,
		RequestsPerIteration: svc.RequestsPerIteration,
	}
	if cfg.RequestsPerIteration <= 0 {
		cfg.RequestsPerIteration = 1
	}

	// Parse scenario steps if present.
	if svc.Steps != "" && svc.Steps != "[]" {
		var steps []config.Step
		if err := json.Unmarshal([]byte(svc.Steps), &steps); err == nil && len(steps) > 0 {
			cfg.Steps = steps
		}
	}

	// Send config to each worker.
	client := &http.Client{Timeout: 10 * time.Second}
	cfgJSON, _ := json.Marshal(cfg)

	var started []string
	var failed []string
	for _, u := range workerURLs {
		resp, err := client.Post(u+"/worker/run", "application/json", bytes.NewReader(cfgJSON))
		if err != nil {
			failed = append(failed, u)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			started = append(started, u)
		} else {
			failed = append(failed, u)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "distributed",
		"started": started,
		"failed":  failed,
		"workers": len(workerURLs),
		"concurrency_per_worker": perWorker,
	})
}

// ---------- Workspaces ----------

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listWorkspaces(w, r)
	case http.MethodPost:
		s.createWorkspace(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkspaceRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/workspaces/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid workspace id", http.StatusBadRequest)
		return
	}

	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "" && r.Method == http.MethodDelete:
		s.deleteWorkspace(w, r, id)
	case action == "services" && r.Method == http.MethodGet:
		s.listWorkspaceServices(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.store.ListWorkspaces()
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaces)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var ws storage.Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if ws.Name == "" || ws.Slug == "" {
		http.Error(w, "name and slug are required", http.StatusBadRequest)
		return
	}
	if err := s.store.CreateWorkspace(&ws); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request, id int64) {
	if defID, err := s.store.DefaultWorkspaceID(); err == nil && id == defID {
		http.Error(w, "the default workspace cannot be deleted", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteWorkspace(id); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listWorkspaceServices(w http.ResponseWriter, r *http.Request, id int64) {
	services, err := s.store.ListServicesByWorkspace(id)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, services)
}

// ---------- plugins ----------

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"protocols":  plugin.Default.ListProtocols(),
		"collectors": plugin.Default.ListCollectors(),
	})
}

// ---------- Templates ----------

var predefinedTemplates = []testTemplate{
	{
		ID: "rest-crud", Name: "REST CRUD API", Category: "API",
		Description: "Tests standard REST endpoints with weighted distribution: GET 70%, POST 20%, PUT 5%, DELETE 5%",
		Service: templateService{
			URL: "https://api.example.com", Method: "GET", Concurrency: 50, Duration: "30s", Timeout: "10s",
			Headers:    map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
			Steps:      `[{"name":"List","url":"https://api.example.com/items","method":"GET","weight":70},{"name":"Create","url":"https://api.example.com/items","method":"POST","body":"{\"name\":\"{{gen.name}}\",\"email\":\"{{gen.email}}\"}","weight":20},{"name":"Update","url":"https://api.example.com/items/{{gen.int100}}","method":"PUT","body":"{\"name\":\"{{gen.name}}\"}","weight":5},{"name":"Delete","url":"https://api.example.com/items/{{gen.int100}}","method":"DELETE","weight":5}]`,
			Assertions: `[{"metric":"p95_latency","operator":"lt","value":500},{"metric":"error_rate","operator":"lt","value":5}]`,
		},
	},
	{
		ID: "auth-flow", Name: "Authentication Flow", Category: "Scenario",
		Description: "Login → Get Token → Access Protected Resource → Logout with session cookies",
		Service: templateService{
			URL: "https://api.example.com/login", Method: "POST", Concurrency: 20, Duration: "30s", Timeout: "15s",
			Headers:     map[string]string{"Content-Type": "application/json"},
			Steps:       `[{"name":"Login","url":"https://api.example.com/login","method":"POST","body":"{\"username\":\"{{gen.email}}\",\"password\":\"test123\"}","extractors":[{"name":"token","source":"body","path":"data.access_token"}]},{"name":"Get Profile","url":"https://api.example.com/profile","method":"GET","headers":{"Authorization":"Bearer {{token}}"},"extractors":[{"name":"user_id","source":"body","path":"data.id"}]},{"name":"Update Profile","url":"https://api.example.com/users/{{user_id}}","method":"PUT","body":"{\"name\":\"{{gen.name}}\"}","headers":{"Authorization":"Bearer {{token}}"}},{"name":"Logout","url":"https://api.example.com/logout","method":"POST","headers":{"Authorization":"Bearer {{token}}"}}]`,
			Assertions:  `[{"metric":"p95_latency","operator":"lt","value":1000},{"metric":"error_rate","operator":"lt","value":2}]`,
			ThinkTimeMs: 500,
		},
	},
	{
		ID: "health-check", Name: "Quick Health Check", Category: "Basic",
		Description: "Simple endpoint availability test with strict latency assertion",
		Service: templateService{
			URL: "https://api.example.com/health", Method: "GET", Concurrency: 5, Duration: "10s", Timeout: "5s",
			Assertions:  `[{"metric":"p95_latency","operator":"lt","value":100},{"metric":"error_rate","operator":"lt","value":1}]`,
			Validations: `[{"type":"status_code","value":"200"}]`,
		},
	},
	{
		ID: "stress-api", Name: "API Stress Test", Category: "Performance",
		Description: "High concurrency stress test with gradual ramp-up to find breaking point",
		Service: templateService{
			URL: "https://api.example.com/endpoint", Method: "GET", Concurrency: 500, Duration: "120s", Timeout: "30s",
			Headers:    map[string]string{"Accept": "application/json"},
			Assertions: `[{"metric":"p95_latency","operator":"lt","value":2000},{"metric":"error_rate","operator":"lt","value":10}]`,
		},
	},
	{
		ID: "file-upload", Name: "File Upload Test", Category: "Upload",
		Description: "Multipart form-data file upload simulation with random data",
		Service: templateService{
			URL: "https://api.example.com/upload", Method: "POST", Concurrency: 10, Duration: "30s", Timeout: "30s",
		},
	},
	{
		ID: "graphql-query", Name: "GraphQL Query Test", Category: "GraphQL",
		Description: "GraphQL endpoint load test with sample query",
		Service: templateService{
			URL: "https://api.example.com/graphql", Method: "POST", Concurrency: 30, Duration: "30s", Timeout: "10s",
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"query":"{ users(limit: 10) { id name email } }"}`,
			Assertions: `[{"metric":"p95_latency","operator":"lt","value":500}]`,
		},
	},
	{
		ID: "websocket-echo", Name: "WebSocket Echo Test", Category: "WebSocket",
		Description: "WebSocket connection and message echo test",
		Service: templateService{
			URL: "wss://echo.websocket.org", Method: "GET", Concurrency: 50, Duration: "30s", Timeout: "10s",
		},
	},
	{
		ID: "realistic-user", Name: "Realistic User Simulation", Category: "Scenario",
		Description: "Simulates real user behavior with think time and varied endpoints",
		Service: templateService{
			URL: "https://api.example.com", Method: "GET", Concurrency: 100, Duration: "60s", Timeout: "15s",
			ThinkTimeMs: 1000, ArrivalRate: 50,
			Steps:      `[{"name":"Browse","url":"https://api.example.com/products","method":"GET","weight":50},{"name":"Search","url":"https://api.example.com/search?q={{gen.word}}","method":"GET","weight":30},{"name":"View Detail","url":"https://api.example.com/products/{{gen.int100}}","method":"GET","weight":15},{"name":"Add to Cart","url":"https://api.example.com/cart","method":"POST","body":"{\"product_id\":{{gen.int100}},\"qty\":1}","weight":5}]`,
			Assertions: `[{"metric":"p95_latency","operator":"lt","value":800},{"metric":"error_rate","operator":"lt","value":3}]`,
		},
	},
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, predefinedTemplates)
}

// ---------- Health / Ready / Version ----------

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    AppVersion,
		"go":         runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"goroutines": runtime.NumGoroutine(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)

	// Database check.
	dbStatus := "ok"
	dbErr := ""
	if err := s.store.Ping(); err != nil {
		dbStatus = "error"
		dbErr = err.Error()
	}

	// Running tests count.
	s.mu.RLock()
	runningTests := len(s.runs)
	s.mu.RUnlock()

	// Memory stats.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// DB stats (best-effort — don't fail health check if this errors).
	var dbStats interface{}
	if st, err := s.store.Stats(); err == nil {
		dbStats = st
	}

	status := http.StatusOK
	overall := "healthy"
	if dbStatus != "ok" {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}

	writeJSON(w, status, map[string]interface{}{
		"status":  overall,
		"version": AppVersion,
		"uptime":  uptime.String(),
		"uptime_seconds": int(uptime.Seconds()),
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"running_tests":  runningTests,
		"database": map[string]interface{}{
			"status": dbStatus,
			"error":  dbErr,
			"stats":  dbStats,
		},
		"memory": map[string]interface{}{
			"alloc_mb":       float64(mem.Alloc) / 1024 / 1024,
			"sys_mb":         float64(mem.Sys) / 1024 / 1024,
			"gc_cycles":      mem.NumGC,
			"heap_objects":   mem.HeapObjects,
		},
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
