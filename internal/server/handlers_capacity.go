package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/prom"
	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/internal/storage"
	"github.com/mertgundoganx/gload/pkg/config"
)

// capacityConfigForService builds a runner config that reproduces the service's
// request shape (URL, method, headers, body, steps, protocol, …). Concurrency
// and duration are left to the probe, and think time is omitted so the probe
// measures true throughput.
func capacityConfigForService(svc *storage.Service) *config.Config {
	timeout, err := time.ParseDuration(svc.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}
	cfg := &config.Config{
		URL:                  svc.URL,
		Method:               strings.ToUpper(svc.Method),
		Body:                 svc.Body,
		Headers:              svc.Headers,
		Timeout:              timeout,
		NoUI:                 true,
		CookieJar:            svc.CookieJar == 1,
		HTTP2:                svc.HTTP2 != 0,
		DisableKeepAlive:     svc.DisableKeepAlive != 0,
		MaxIdleConns:         svc.MaxIdleConns,
		DNSCacheEnabled:      svc.DNSCache != 0,
		RequestsPerIteration: 1,
		ContentType:          svc.ContentType,
		Protocol:             svc.Protocol,
	}
	if svc.Steps != "" && svc.Steps != "[]" {
		var steps []config.Step
		if json.Unmarshal([]byte(svc.Steps), &steps) == nil && len(steps) > 0 {
			cfg.Steps = steps
		}
	}
	if svc.DataSource != "" && svc.DataSource != "[]" {
		var ds []map[string]string
		if json.Unmarshal([]byte(svc.DataSource), &ds) == nil && len(ds) > 0 {
			cfg.DataSource = ds
		}
	}
	if svc.Validations != "" && svc.Validations != "[]" {
		var vals []config.Validation
		if json.Unmarshal([]byte(svc.Validations), &vals) == nil && len(vals) > 0 {
			cfg.Validations = vals
		}
	}
	if svc.FormFields != "" && svc.FormFields != "[]" {
		var ff []config.FormField
		if json.Unmarshal([]byte(svc.FormFields), &ff) == nil && len(ff) > 0 {
			cfg.FormFields = ff
		}
	}
	if svc.ProtocolConfig != "" && svc.ProtocolConfig != "{}" {
		var pc map[string]string
		if json.Unmarshal([]byte(svc.ProtocolConfig), &pc) == nil {
			cfg.ProtocolConfig = pc
		}
	}
	return cfg
}

// startCapacityProbe launches an adaptive capacity probe for the service. It
// runs asynchronously (like a normal test), streams live metrics via the usual
// SSE endpoint, and stores the CapacityResult for later retrieval.
func (s *Server) startCapacityProbe(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// The capacity probe is deliberately opinionated — it uses one curated,
	// adaptive methodology with no user-facing knobs. It auto-ramps to the knee
	// and stops; there's nothing to tune.
	s.mu.Lock()
	if _, running := s.runs[id]; running {
		s.mu.Unlock()
		http.Error(w, "test already running", http.StatusConflict)
		return
	}

	cfg := capacityConfigForService(svc)
	ctx, cancel := context.WithCancel(context.Background())
	r := runner.New(cfg)
	// Duration is unknown (adaptive); use a nominal value for the run registry.
	s.runs[id] = &runState{runner: r, cancel: cancel, duration: 0, kind: "capacity"}
	prom.Global.SetRunning(len(s.runs))
	s.mu.Unlock()

	svcName := svc.Name

	go func() {
		res := r.RunCapacityProbe(ctx, runner.CapacityConfig{})
		r.Close()

		if data, mErr := json.Marshal(res); mErr == nil {
			if sErr := s.store.SaveCapacityResult(id, string(data)); sErr != nil {
				logger.Error("failed to save capacity result", logger.Fields("service_id", id, "error", sErr.Error()))
			}
		}

		logger.Info("capacity probe complete",
			logger.Fields("service_id", id, "knee", res.KneeConcurrency, "max_rps", res.MaxRPS, "reason", res.Reason))

		go s.broadcast.send("test_completed", map[string]interface{}{
			"service_id": id, "service_name": svcName, "status": "pass",
		})

		s.mu.Lock()
		delete(s.runs, id)
		prom.Global.SetRunning(len(s.runs))
		s.mu.Unlock()
	}()

	go s.broadcast.send("test_started", map[string]interface{}{"service_id": id, "service_name": svcName})
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// getCapacityProbe returns the service's capacity-probe run history (newest
// first) as a JSON array of {id, created_at, result}.
func (s *Server) getCapacityProbe(w http.ResponseWriter, _ *http.Request, id int64) {
	runs, err := s.store.ListCapacityRuns(id)
	if err != nil {
		dbError(w, err)
		return
	}
	type item struct {
		ID        int64           `json:"id"`
		CreatedAt string          `json:"created_at"`
		Result    json.RawMessage `json:"result"`
	}
	out := make([]item, 0, len(runs))
	for _, r := range runs {
		out = append(out, item{ID: r.ID, CreatedAt: r.CreatedAt.Format(time.RFC3339), Result: json.RawMessage(r.Result)})
	}
	writeJSON(w, http.StatusOK, out)
}

// getCapacityReport renders a shareable, print-friendly HTML capacity report for
// a run (?run=<id>, or the latest).
func (s *Server) getCapacityReport(w http.ResponseWriter, r *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	runs, err := s.store.ListCapacityRuns(id)
	if err != nil {
		dbError(w, err)
		return
	}
	if len(runs) == 0 {
		http.Error(w, "no capacity result yet", http.StatusNotFound)
		return
	}
	chosen := runs[0]
	if rid := r.URL.Query().Get("run"); rid != "" {
		if v, e := strconv.ParseInt(rid, 10, 64); e == nil {
			for _, run := range runs {
				if run.ID == v {
					chosen = run
					break
				}
			}
		}
	}
	var res runner.CapacityResult
	if json.Unmarshal([]byte(chosen.Result), &res) != nil {
		http.Error(w, "invalid capacity result", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(generateCapacityReportHTML(svc, res, chosen.CreatedAt)))
}
