package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/prom"
	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/pkg/config"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// ---------- Run / Stop / Stream ----------

func (s *Server) runService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	s.mu.Lock()
	if _, running := s.runs[id]; running {
		s.mu.Unlock()
		http.Error(w, "test already running", http.StatusConflict)
		return
	}

	dur, err := time.ParseDuration(svc.Duration)
	if err != nil {
		s.mu.Unlock()
		http.Error(w, "invalid duration: "+err.Error(), http.StatusBadRequest)
		return
	}
	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Concurrency:          svc.Concurrency,
		Duration:             dur,
		Timeout:              timeout,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		HTTP2:                svc.HTTP2 != 0,
		DisableKeepAlive:     svc.DisableKeepAlive != 0,
		MaxIdleConns:         svc.MaxIdleConns,
		DNSCacheEnabled:      svc.DNSCache != 0,
		WarmupDuration:       time.Duration(svc.WarmupSeconds) * time.Second,
		ThinkTime:            time.Duration(svc.ThinkTimeMs) * time.Millisecond,
		ThinkTimeMax:         time.Duration(svc.ThinkTimeMaxMs) * time.Millisecond,
		ArrivalRate:          svc.ArrivalRate,
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

	// Parse dynamic data source if present.
	if svc.DataSource != "" && svc.DataSource != "[]" {
		var ds []map[string]string
		if err := json.Unmarshal([]byte(svc.DataSource), &ds); err == nil && len(ds) > 0 {
			cfg.DataSource = ds
		}
	}

	// Parse validations if present.
	if svc.Validations != "" && svc.Validations != "[]" {
		var vals []config.Validation
		if err := json.Unmarshal([]byte(svc.Validations), &vals); err == nil && len(vals) > 0 {
			cfg.Validations = vals
		}
	}

	// Parse multipart form fields if present.
	cfg.ContentType = svc.ContentType
	if svc.FormFields != "" && svc.FormFields != "[]" {
		var ff []config.FormField
		if err := json.Unmarshal([]byte(svc.FormFields), &ff); err == nil && len(ff) > 0 {
			cfg.FormFields = ff
		}
	}

	// Parse protocol config if present.
	cfg.Protocol = svc.Protocol
	if svc.ProtocolConfig != "" && svc.ProtocolConfig != "{}" {
		var pc map[string]string
		if err := json.Unmarshal([]byte(svc.ProtocolConfig), &pc); err == nil {
			cfg.ProtocolConfig = pc
		}
	}

	// Parse assertions.
	var assertions []config.Assertion
	if svc.Assertions != "" && svc.Assertions != "[]" {
		json.Unmarshal([]byte(svc.Assertions), &assertions)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := runner.New(cfg)
	s.runs[id] = &runState{runner: r, cancel: cancel, duration: dur}
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()

	svcName := svc.Name
	svcURL := svc.URL

	go func() {
		r.Run(ctx)
		r.Close()
		snap := r.Metrics.Snapshot()

		// Save result to storage.
		tr := snapshotToTestResult(snap)

		// Evaluate assertions.
		if len(assertions) > 0 {
			tr.Status, tr.AssertionResults = evaluateAssertions(snap, assertions)
		}

		// Attach run config.
		runCfg := RunConfigData{
			Type:        "manual",
			Concurrency: cfg.Concurrency,
			Duration:    svc.Duration,
			RPS:         cfg.RPS,
			ArrivalRate: cfg.ArrivalRate,
			ThinkTimeMs: svc.ThinkTimeMs,
		}
		runCfgJSON, _ := json.Marshal(runCfg)
		tr.RunConfig = string(runCfgJSON)

		if saveErr := s.store.SaveTestResult(id, &tr); saveErr != nil {
			logger.Error("failed to save test result", logger.Fields("service_id", id, "error", saveErr.Error()))
		}

		// Update Prometheus metrics.
		errRate := 0.0
		if tr.TotalReqs > 0 {
			errRate = float64(tr.Errors) / float64(tr.TotalReqs) * 100
		}
		prom.Global.RecordTestComplete(tr.Status != "fail", tr.TotalReqs, tr.Errors, tr.RPS, tr.AvgLatencyMs, tr.P95LatencyMs, errRate)

		// Send notifications.
		go s.sendNotification(svcName, svcURL, &tr)

		// Broadcast test completion.
		go s.broadcast.send("test_completed", map[string]interface{}{
			"service_id": id, "service_name": svcName, "status": tr.Status,
			"rps": tr.RPS, "avg_latency_ms": tr.AvgLatencyMs,
		})

		s.mu.Lock()
		delete(s.runs, id)
		prom.Global.SetRunning(len(s.runs))
		s.mu.Unlock()
	}()

	go s.broadcast.send("test_started", map[string]interface{}{"service_id": id, "service_name": svcName})
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) stopService(w http.ResponseWriter, _ *http.Request, id int64) {
	s.mu.Lock()
	rs, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "no running test", http.StatusNotFound)
		return
	}
	rs.cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) streamService(w http.ResponseWriter, r *http.Request, id int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			rs, running := s.runs[id]
			s.mu.RUnlock()

			if running {
				snap := rs.runner.Metrics.Snapshot()
				data, _ := json.Marshal(snapshotJSONWithProgress(snap, rs.duration))
				fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
				flusher.Flush()
			} else {
				// Test finished — send final snapshot from storage.
				last, err := s.store.GetLastResult(id)
				if err == nil && last != nil {
					dur := time.Duration(last.DurationMs * float64(time.Millisecond))
					sd := testResultToSnapshotWithProgress(last, dur)
					data, _ := json.Marshal(sd)
					fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
				} else {
					fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				}
				flusher.Flush()
				return
			}
		}
	}
}

func (s *Server) wsService(w http.ResponseWriter, r *http.Request, id int64) {
	conn, err := websocket.Accept(w, r, wsAcceptOptions())
	if err != nil {
		http.Error(w, "websocket upgrade failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	ctx := conn.CloseRead(r.Context())

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			rs, running := s.runs[id]
			s.mu.RUnlock()

			if running {
				snap := rs.runner.Metrics.Snapshot()
				msg := map[string]interface{}{
					"event": "metrics",
					"data":  snapshotJSONWithProgress(snap, rs.duration),
				}
				wsjson.Write(ctx, conn, msg)
			} else {
				last, err := s.store.GetLastResult(id)
				if err == nil && last != nil {
					dur := time.Duration(last.DurationMs * float64(time.Millisecond))
					msg := map[string]interface{}{
						"event": "done",
						"data":  testResultToSnapshotWithProgress(last, dur),
					}
					wsjson.Write(ctx, conn, msg)
				} else {
					wsjson.Write(ctx, conn, map[string]string{"event": "done"})
				}
				return
			}
		}
	}
}

// ---------- Run Profile ----------

func (s *Server) runProfile(w http.ResponseWriter, r *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name         string `json:"name"`
		ProfileIndex int    `json:"profile_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	var profiles []profileConfig
	if svc.Profiles == "" || svc.Profiles == "[]" {
		http.Error(w, "no profiles configured", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal([]byte(svc.Profiles), &profiles); err != nil {
		http.Error(w, "invalid profiles json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Select by name when provided, otherwise fall back to profile_index.
	idx := body.ProfileIndex
	if body.Name != "" {
		idx = -1
		for i, p := range profiles {
			if strings.EqualFold(p.Name, body.Name) {
				idx = i
				break
			}
		}
		if idx == -1 {
			http.Error(w, fmt.Sprintf("profile %q not found", body.Name), http.StatusBadRequest)
			return
		}
	}
	if idx < 0 || idx >= len(profiles) {
		http.Error(w, "invalid profile_index", http.StatusBadRequest)
		return
	}

	profile := profiles[idx]

	s.mu.Lock()
	if _, running := s.runs[id]; running {
		s.mu.Unlock()
		http.Error(w, "test already running", http.StatusConflict)
		return
	}

	dur, err := time.ParseDuration(profile.Duration)
	if err != nil {
		s.mu.Unlock()
		http.Error(w, "invalid profile duration: "+err.Error(), http.StatusBadRequest)
		return
	}
	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Concurrency:          profile.Concurrency,
		Duration:             dur,
		Timeout:              timeout,
		RPS:                  profile.RPS,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		HTTP2:                svc.HTTP2 != 0,
		DisableKeepAlive:     svc.DisableKeepAlive != 0,
		MaxIdleConns:         svc.MaxIdleConns,
		DNSCacheEnabled:      svc.DNSCache != 0,
		WarmupDuration:       time.Duration(svc.WarmupSeconds) * time.Second,
		ThinkTime:            time.Duration(svc.ThinkTimeMs) * time.Millisecond,
		ThinkTimeMax:         time.Duration(svc.ThinkTimeMaxMs) * time.Millisecond,
		ArrivalRate:          svc.ArrivalRate,
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

	// Parse dynamic data source if present.
	if svc.DataSource != "" && svc.DataSource != "[]" {
		var ds []map[string]string
		if err := json.Unmarshal([]byte(svc.DataSource), &ds); err == nil && len(ds) > 0 {
			cfg.DataSource = ds
		}
	}

	// Parse validations if present.
	if svc.Validations != "" && svc.Validations != "[]" {
		var vals []config.Validation
		if err := json.Unmarshal([]byte(svc.Validations), &vals); err == nil && len(vals) > 0 {
			cfg.Validations = vals
		}
	}

	// Parse multipart form fields if present.
	cfg.ContentType = svc.ContentType
	if svc.FormFields != "" && svc.FormFields != "[]" {
		var ff []config.FormField
		if err := json.Unmarshal([]byte(svc.FormFields), &ff); err == nil && len(ff) > 0 {
			cfg.FormFields = ff
		}
	}

	// Parse assertions.
	var assertions []config.Assertion
	if svc.Assertions != "" && svc.Assertions != "[]" {
		json.Unmarshal([]byte(svc.Assertions), &assertions)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rn := runner.New(cfg)
	s.runs[id] = &runState{runner: rn, cancel: cancel, duration: dur}
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()

	go func() {
		rn.Run(ctx)
		snap := rn.Metrics.Snapshot()

		tr := snapshotToTestResult(snap)

		// Evaluate assertions.
		if len(assertions) > 0 {
			tr.Status, tr.AssertionResults = evaluateAssertions(snap, assertions)
		}

		// Attach run config.
		runCfg := RunConfigData{
			Type:        "profile",
			ProfileName: profile.Name,
			Concurrency: profile.Concurrency,
			Duration:    profile.Duration,
			RPS:         profile.RPS,
		}
		runCfgJSON, _ := json.Marshal(runCfg)
		tr.RunConfig = string(runCfgJSON)

		if saveErr := s.store.SaveTestResult(id, &tr); saveErr != nil {
			logger.Error("failed to save test result", logger.Fields("service_id", id, "profile", profile.Name, "error", saveErr.Error()))
		}

		// Update Prometheus metrics.
		errRate := 0.0
		if tr.TotalReqs > 0 {
			errRate = float64(tr.Errors) / float64(tr.TotalReqs) * 100
		}
		prom.Global.RecordTestComplete(tr.Status != "fail", tr.TotalReqs, tr.Errors, tr.RPS, tr.AvgLatencyMs, tr.P95LatencyMs, errRate)

		// Send notifications.
		go s.sendNotification(svc.Name, svc.URL, &tr)

		// Broadcast test completion.
		go s.broadcast.send("test_completed", map[string]interface{}{
			"service_id": id, "service_name": svc.Name, "status": tr.Status,
			"rps": tr.RPS, "avg_latency_ms": tr.AvgLatencyMs,
		})

		s.mu.Lock()
		delete(s.runs, id)
		prom.Global.SetRunning(len(s.runs))
		s.mu.Unlock()
	}()

	go s.broadcast.send("test_started", map[string]interface{}{"service_id": id, "service_name": svc.Name})
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "profile": profile.Name})
}

// ---------- Load Patterns ----------

// predefinedPatterns are ordered from lightest to heaviest, covering the full
// testing lifecycle: sanity → expected load → scaling → burst → breaking point
// → endurance. Each stage ramps the virtual-user count linearly from the
// previous stage's target to its own over the stage duration (a stage with the
// same target holds steady), so the descriptions match what actually runs.
var predefinedPatterns = []loadPattern{
	{
		Name:        "Smoke Test",
		Description: "A tiny load (5 users) to confirm the endpoint works before running anything heavy — your pre-flight sanity check.",
		Stages: []patternStage{
			{Duration: "15s", Target: 5, RPS: 0}, // ease up to 5
			{Duration: "45s", Target: 5, RPS: 0}, // hold
		},
	},
	{
		Name:        "Steady Load",
		Description: "Ramps to a production-like load and holds it for 10 minutes — confirms the system comfortably handles expected everyday traffic.",
		Stages: []patternStage{
			{Duration: "60s", Target: 100, RPS: 0},  // ramp 0 → 100
			{Duration: "600s", Target: 100, RPS: 0}, // hold expected load
			{Duration: "30s", Target: 0, RPS: 0},    // ramp down
		},
	},
	{
		Name:        "Ramp Up",
		Description: "A smooth linear ramp from zero to 500 users, then held — shows how latency and throughput evolve as load grows and how quickly the system provisions.",
		Stages: []patternStage{
			{Duration: "300s", Target: 500, RPS: 0}, // linear ramp 0 → 500 over 5 min
			{Duration: "120s", Target: 500, RPS: 0}, // hold at target
			{Duration: "30s", Target: 0, RPS: 0},    // ramp down
		},
	},
	{
		Name:        "Spike Test",
		Description: "Baseline traffic, then sudden spikes to 1000 users and back (flash sale, viral event) — tests burst handling and how fast it recovers. Two spikes check repeatability.",
		Stages: []patternStage{
			{Duration: "30s", Target: 50, RPS: 0},    // ramp to baseline
			{Duration: "10s", Target: 1000, RPS: 0},  // spike up (fast ramp)
			{Duration: "120s", Target: 1000, RPS: 0}, // hold the spike
			{Duration: "20s", Target: 50, RPS: 0},    // drop back
			{Duration: "60s", Target: 50, RPS: 0},    // recover at baseline
			{Duration: "10s", Target: 1000, RPS: 0},  // second spike
			{Duration: "120s", Target: 1000, RPS: 0}, // hold
			{Duration: "30s", Target: 0, RPS: 0},     // ramp down
		},
	},
	{
		Name:        "Stress Test",
		Description: "Steps the load up level by level (200 → 500 → 1000 → 2000), holding at each, to find the breaking point and watch degradation set in.",
		Stages: []patternStage{
			{Duration: "30s", Target: 200, RPS: 0},
			{Duration: "60s", Target: 200, RPS: 0},
			{Duration: "30s", Target: 500, RPS: 0},
			{Duration: "60s", Target: 500, RPS: 0},
			{Duration: "30s", Target: 1000, RPS: 0},
			{Duration: "60s", Target: 1000, RPS: 0},
			{Duration: "30s", Target: 2000, RPS: 0},
			{Duration: "60s", Target: 2000, RPS: 0},
			{Duration: "30s", Target: 0, RPS: 0},
		},
	},
	{
		Name:        "Soak Test",
		Description: "Holds a moderate load for 30 minutes — surfaces slow problems: memory leaks, connection-pool exhaustion, and gradual degradation over time.",
		Stages: []patternStage{
			{Duration: "60s", Target: 100, RPS: 0},   // ramp up
			{Duration: "1800s", Target: 100, RPS: 0}, // 30 min steady
			{Duration: "60s", Target: 0, RPS: 0},     // ramp down
		},
	},
}

func (s *Server) handlePatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, predefinedPatterns)
}

func (s *Server) runPattern(w http.ResponseWriter, r *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var body struct {
		PatternName string         `json:"pattern_name"`
		Stages      []patternStage `json:"stages"`
		OpenModel   bool           `json:"open_model"` // stage targets are arrival rate (req/s), not concurrency
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	var stages []patternStage

	if len(body.Stages) > 0 {
		stages = body.Stages
	} else if body.PatternName != "" {
		for _, p := range predefinedPatterns {
			if p.Name == body.PatternName {
				stages = p.Stages
				break
			}
		}
		if stages == nil {
			http.Error(w, "unknown pattern: "+body.PatternName, http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "pattern_name or stages required", http.StatusBadRequest)
		return
	}

	// Convert pattern stages to config stages.
	var configStages []config.Stage
	var totalDuration time.Duration
	for _, ps := range stages {
		dur, err := time.ParseDuration(ps.Duration)
		if err != nil {
			http.Error(w, "invalid stage duration: "+ps.Duration, http.StatusBadRequest)
			return
		}
		configStages = append(configStages, config.Stage{
			Duration: dur,
			Target:   ps.Target,
			RPS:      ps.RPS,
		})
		totalDuration += dur
	}

	s.mu.Lock()
	if _, running := s.runs[id]; running {
		s.mu.Unlock()
		http.Error(w, "test already running", http.StatusConflict)
		return
	}

	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Concurrency:          0, // stages control concurrency
		Duration:             totalDuration,
		Timeout:              timeout,
		Stages:               configStages,
		OpenModel:            body.OpenModel,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		HTTP2:                svc.HTTP2 != 0,
		DisableKeepAlive:     svc.DisableKeepAlive != 0,
		MaxIdleConns:         svc.MaxIdleConns,
		DNSCacheEnabled:      svc.DNSCache != 0,
		WarmupDuration:       time.Duration(svc.WarmupSeconds) * time.Second,
		ThinkTime:            time.Duration(svc.ThinkTimeMs) * time.Millisecond,
		ThinkTimeMax:         time.Duration(svc.ThinkTimeMaxMs) * time.Millisecond,
		ArrivalRate:          svc.ArrivalRate,
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

	// Parse dynamic data source if present.
	if svc.DataSource != "" && svc.DataSource != "[]" {
		var ds []map[string]string
		if err := json.Unmarshal([]byte(svc.DataSource), &ds); err == nil && len(ds) > 0 {
			cfg.DataSource = ds
		}
	}

	// Parse validations if present.
	if svc.Validations != "" && svc.Validations != "[]" {
		var vals []config.Validation
		if err := json.Unmarshal([]byte(svc.Validations), &vals); err == nil && len(vals) > 0 {
			cfg.Validations = vals
		}
	}

	// Parse multipart form fields if present.
	cfg.ContentType = svc.ContentType
	if svc.FormFields != "" && svc.FormFields != "[]" {
		var ff []config.FormField
		if err := json.Unmarshal([]byte(svc.FormFields), &ff); err == nil && len(ff) > 0 {
			cfg.FormFields = ff
		}
	}

	// Parse assertions.
	var assertions []config.Assertion
	if svc.Assertions != "" && svc.Assertions != "[]" {
		json.Unmarshal([]byte(svc.Assertions), &assertions)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rn := runner.New(cfg)
	s.runs[id] = &runState{runner: rn, cancel: cancel, duration: totalDuration}
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()

	patternLabel := body.PatternName
	if patternLabel == "" {
		patternLabel = "custom"
	}

	go func() {
		rn.Run(ctx)
		snap := rn.Metrics.Snapshot()

		tr := snapshotToTestResult(snap)

		// Evaluate assertions.
		if len(assertions) > 0 {
			tr.Status, tr.AssertionResults = evaluateAssertions(snap, assertions)
		}

		// Attach run config.
		stagesJSON, _ := json.Marshal(stages)
		runCfg := RunConfigData{
			Type:        "pattern",
			PatternName: patternLabel,
			Concurrency: 0,
			Duration:    totalDuration.String(),
			Stages:      string(stagesJSON),
			OpenModel:   body.OpenModel,
		}
		runCfgJSON, _ := json.Marshal(runCfg)
		tr.RunConfig = string(runCfgJSON)

		if saveErr := s.store.SaveTestResult(id, &tr); saveErr != nil {
			logger.Error("failed to save test result", logger.Fields("service_id", id, "pattern", patternLabel, "error", saveErr.Error()))
		}

		// Update Prometheus metrics.
		errRate := 0.0
		if tr.TotalReqs > 0 {
			errRate = float64(tr.Errors) / float64(tr.TotalReqs) * 100
		}
		prom.Global.RecordTestComplete(tr.Status != "fail", tr.TotalReqs, tr.Errors, tr.RPS, tr.AvgLatencyMs, tr.P95LatencyMs, errRate)

		// Send notifications.
		go s.sendNotification(svc.Name, svc.URL, &tr)

		// Broadcast test completion.
		go s.broadcast.send("test_completed", map[string]interface{}{
			"service_id": id, "service_name": svc.Name, "status": tr.Status,
			"rps": tr.RPS, "avg_latency_ms": tr.AvgLatencyMs,
		})

		s.mu.Lock()
		delete(s.runs, id)
		prom.Global.SetRunning(len(s.runs))
		s.mu.Unlock()
	}()

	go s.broadcast.send("test_started", map[string]interface{}{"service_id": id, "service_name": svc.Name})
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "pattern": patternLabel})
}

// ---------- Queue ----------

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getQueue(w, r)
	case http.MethodPost:
		s.addToQueue(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleQueueRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/queue/")
	if path == "add" && r.Method == http.MethodPost {
		s.addToQueue(w, r)
		return
	}
	if path == "clear" && r.Method == http.MethodPost {
		s.clearQueue(w, r)
		return
	}
	if path == "reorder" && r.Method == http.MethodPost {
		s.reorderQueue(w, r)
		return
	}
	// DELETE /api/queue/{id} — remove by stable item id (not position).
	if r.Method == http.MethodDelete {
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			http.Error(w, "invalid queue id", http.StatusBadRequest)
			return
		}
		s.removeFromQueue(w, r, id)
		return
	}
	http.NotFound(w, r)
}

// startQueueProcessor launches the queue worker if it isn't already running.
func (s *Server) startQueueProcessor() {
	s.queueMu.Lock()
	if !s.queueRunning {
		s.queueRunning = true
		go s.processQueue()
	}
	s.queueMu.Unlock()
}

func (s *Server) addToQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceID int64 `json:"service_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify the service exists.
	svc, err := s.store.GetService(body.ServiceID)
	if err != nil || svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Prevent silent duplicate enqueues of the same service.
	if n, _ := s.store.CountQueueForService(body.ServiceID); n > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Service is already in the queue."})
		return
	}

	id, err := s.store.EnqueueTest(body.ServiceID)
	if err != nil {
		dbError(w, err)
		return
	}
	s.startQueueProcessor()
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) getQueue(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.ListQueue()
	if err != nil {
		dbError(w, err)
		return
	}

	s.queueMu.Lock()
	running := s.queueRunning
	s.queueMu.Unlock()

	// "Currently running" reflects any in-flight test (queue or manual).
	var currentID int64
	s.mu.RLock()
	for id := range s.runs {
		currentID = id
		break
	}
	runningCount := len(s.runs)
	s.mu.RUnlock()

	var currentEntry *queueEntry
	if currentID > 0 {
		svc, _ := s.store.GetService(currentID)
		name := fmt.Sprintf("Service #%d", currentID)
		if svc != nil {
			name = svc.Name
		}
		currentEntry = &queueEntry{ServiceID: currentID, Name: name}
	}

	pending := make([]queueEntry, 0, len(items))
	for _, it := range items {
		svc, _ := s.store.GetService(it.ServiceID)
		name := fmt.Sprintf("Service #%d", it.ServiceID)
		if svc != nil {
			name = svc.Name
		}
		pending = append(pending, queueEntry{ID: it.ID, ServiceID: it.ServiceID, Name: name})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":   pending,
		"current": currentEntry,
		"running": running || runningCount > 0,
	})
}

func (s *Server) clearQueue(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.ClearQueue(); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (s *Server) removeFromQueue(w http.ResponseWriter, _ *http.Request, id int64) {
	if err := s.store.RemoveQueueItem(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) reorderQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.ReorderQueue(body.IDs); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) processQueue() {
	for {
		item, err := s.store.PopQueue()
		if err != nil {
			logger.Error("queue: pop failed", logger.Fields("error", err.Error()))
		}
		if item == nil {
			s.queueMu.Lock()
			s.queueRunning = false
			s.queueMu.Unlock()
			return
		}
		s.runQueuedTest(item.ServiceID, "queue")
	}
}

func (s *Server) runQueuedTest(serviceID int64, runType ...string) {
	svc, err := s.store.GetService(serviceID)
	if err != nil || svc == nil {
		logger.Warn("queue: service not found, skipping", logger.Fields("service_id", serviceID))
		return
	}

	dur, err := time.ParseDuration(svc.Duration)
	if err != nil {
		logger.Error("queue: invalid duration", logger.Fields("service_id", serviceID, "error", err.Error()))
		return
	}
	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Concurrency:          svc.Concurrency,
		Duration:             dur,
		Timeout:              timeout,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		HTTP2:                svc.HTTP2 != 0,
		DisableKeepAlive:     svc.DisableKeepAlive != 0,
		MaxIdleConns:         svc.MaxIdleConns,
		DNSCacheEnabled:      svc.DNSCache != 0,
		WarmupDuration:       time.Duration(svc.WarmupSeconds) * time.Second,
		ThinkTime:            time.Duration(svc.ThinkTimeMs) * time.Millisecond,
		ThinkTimeMax:         time.Duration(svc.ThinkTimeMaxMs) * time.Millisecond,
		ArrivalRate:          svc.ArrivalRate,
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

	// Parse dynamic data source if present.
	if svc.DataSource != "" && svc.DataSource != "[]" {
		var ds []map[string]string
		if err := json.Unmarshal([]byte(svc.DataSource), &ds); err == nil && len(ds) > 0 {
			cfg.DataSource = ds
		}
	}

	// Parse validations if present.
	if svc.Validations != "" && svc.Validations != "[]" {
		var vals []config.Validation
		if err := json.Unmarshal([]byte(svc.Validations), &vals); err == nil && len(vals) > 0 {
			cfg.Validations = vals
		}
	}

	// Parse multipart form fields if present.
	cfg.ContentType = svc.ContentType
	if svc.FormFields != "" && svc.FormFields != "[]" {
		var ff []config.FormField
		if err := json.Unmarshal([]byte(svc.FormFields), &ff); err == nil && len(ff) > 0 {
			cfg.FormFields = ff
		}
	}

	// Parse protocol config if present.
	cfg.Protocol = svc.Protocol
	if svc.ProtocolConfig != "" && svc.ProtocolConfig != "{}" {
		var pc map[string]string
		if err := json.Unmarshal([]byte(svc.ProtocolConfig), &pc); err == nil {
			cfg.ProtocolConfig = pc
		}
	}

	// Parse assertions.
	var assertions []config.Assertion
	if svc.Assertions != "" && svc.Assertions != "[]" {
		json.Unmarshal([]byte(svc.Assertions), &assertions)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := runner.New(cfg)

	s.mu.Lock()
	s.runs[serviceID] = &runState{runner: r, cancel: cancel, duration: dur}
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()

	go s.broadcast.send("test_started", map[string]interface{}{"service_id": serviceID, "service_name": svc.Name})

	r.Run(ctx)
	r.Close()
	snap := r.Metrics.Snapshot()

	tr := snapshotToTestResult(snap)

	// Evaluate assertions.
	if len(assertions) > 0 {
		tr.Status, tr.AssertionResults = evaluateAssertions(snap, assertions)
	}

	// Attach run config.
	cfgType := "queue"
	if len(runType) > 0 && runType[0] != "" {
		cfgType = runType[0]
	}
	runCfg := RunConfigData{
		Type:        cfgType,
		Concurrency: cfg.Concurrency,
		Duration:    svc.Duration,
		RPS:         cfg.RPS,
		ArrivalRate: cfg.ArrivalRate,
		ThinkTimeMs: svc.ThinkTimeMs,
	}
	runCfgJSON, _ := json.Marshal(runCfg)
	tr.RunConfig = string(runCfgJSON)

	if saveErr := s.store.SaveTestResult(serviceID, &tr); saveErr != nil {
		logger.Error("queue: failed to save test result", logger.Fields("service_id", serviceID, "error", saveErr.Error()))
	}

	// Update Prometheus metrics.
	errRate := 0.0
	if tr.TotalReqs > 0 {
		errRate = float64(tr.Errors) / float64(tr.TotalReqs) * 100
	}
	prom.Global.RecordTestComplete(tr.Status != "fail", tr.TotalReqs, tr.Errors, tr.RPS, tr.AvgLatencyMs, tr.P95LatencyMs, errRate)

	// Send notifications.
	go s.sendNotification(svc.Name, svc.URL, &tr)

	// Broadcast test completion.
	go s.broadcast.send("test_completed", map[string]interface{}{
		"service_id": serviceID, "service_name": svc.Name, "status": tr.Status,
		"rps": tr.RPS, "avg_latency_ms": tr.AvgLatencyMs,
	})

	s.mu.Lock()
	delete(s.runs, serviceID)
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()
}

func (s *Server) handleBulkQueueAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ServiceIDs []int64 `json:"service_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	added := 0
	for _, id := range body.ServiceIDs {
		// Skip services that don't exist or are already queued.
		if svc, _ := s.store.GetService(id); svc == nil {
			continue
		}
		if n, _ := s.store.CountQueueForService(id); n > 0 {
			continue
		}
		if _, err := s.store.EnqueueTest(id); err == nil {
			added++
		}
	}
	if added > 0 {
		s.startQueueProcessor()
	}
	items, _ := s.store.ListQueue()
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "queue_length": len(items)})
}

// ResumeQueue restarts the queue processor if there are pending items left
// from a previous run (the queue is persisted across restarts).
func (s *Server) ResumeQueue() {
	items, err := s.store.ListQueue()
	if err != nil {
		logger.Error("queue: failed to load persisted queue", logger.Fields("error", err.Error()))
		return
	}
	if len(items) > 0 {
		logger.Info("queue: resuming persisted queue", logger.Fields("pending", len(items)))
		s.startQueueProcessor()
	}
}
