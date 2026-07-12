package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	ghub "github.com/mertgundoganx/gload/internal/github"
	"github.com/mertgundoganx/gload/internal/junit"
	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/report"
	"github.com/mertgundoganx/gload/internal/storage"
)

func (s *Server) resultsService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	last, err := s.store.GetLastResult(id)
	if err != nil || last == nil {
		http.Error(w, "no results yet", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, testResultToSnapshot(last))
}

func (s *Server) historyService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	results, err := s.store.ListResults(id, 50)
	if err != nil {
		dbError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// ---------- Export Results (CSV/JSON) ----------

func (s *Server) exportResults(w http.ResponseWriter, r *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	results, err := s.store.ListResults(id, 1000)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(svc.Name+"-results.csv"))

		cw := csv.NewWriter(w)
		defer cw.Flush()
		_ = cw.Write([]string{"id", "date", "duration_ms", "total_reqs", "errors", "rps", "avg_latency_ms", "p50_latency_ms", "p95_latency_ms", "p99_latency_ms", "min_latency_ms", "max_latency_ms", "status", "note"})
		for _, res := range results {
			_ = cw.Write([]string{
				strconv.FormatInt(res.ID, 10),
				res.CreatedAt.Format("2006-01-02 15:04:05"),
				strconv.FormatFloat(res.DurationMs, 'f', 1, 64),
				strconv.Itoa(res.TotalReqs),
				strconv.Itoa(res.Errors),
				strconv.FormatFloat(res.RPS, 'f', 2, 64),
				strconv.FormatFloat(res.AvgLatencyMs, 'f', 2, 64),
				strconv.FormatFloat(res.P50LatencyMs, 'f', 2, 64),
				strconv.FormatFloat(res.P95LatencyMs, 'f', 2, 64),
				strconv.FormatFloat(res.P99LatencyMs, 'f', 2, 64),
				strconv.FormatFloat(res.MinLatencyMs, 'f', 2, 64),
				strconv.FormatFloat(res.MaxLatencyMs, 'f', 2, 64),
				csvSafe(res.Status),
				csvSafe(res.Note),
			})
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", contentDisposition(svc.Name+"-results.json"))
		json.NewEncoder(w).Encode(results)
	}
}

// csvSafe neutralizes spreadsheet formula injection by prefixing values that
// begin with a formula trigger character with a single quote.
func csvSafe(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	}
	return v
}

// contentDisposition builds a safe Content-Disposition header, stripping any
// characters from the filename that could be used for header injection.
func contentDisposition(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf("attachment; filename=%q", name)
}

func (s *Server) reportService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	last, err := s.store.GetLastResult(id)
	if err != nil || last == nil {
		http.Error(w, "no results yet", http.StatusNotFound)
		return
	}

	snap := testResultToMetricsSnapshot(last)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf bytes.Buffer
	if err := report.Generate(&buf, snap, svc.Name, svc.URL, svc.Method, svc.Concurrency, last.RunConfig); err != nil {
		logger.Error("report generation failed", logger.Fields("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

func (s *Server) pdfService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	last, err := s.store.GetLastResult(id)
	if err != nil || last == nil {
		http.Error(w, "no results yet", http.StatusNotFound)
		return
	}

	snap := testResultToMetricsSnapshot(last)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf bytes.Buffer
	if err := report.Generate(&buf, snap, svc.Name, svc.URL, svc.Method, svc.Concurrency, last.RunConfig); err != nil {
		logger.Error("report generation failed", logger.Fields("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Inject auto-print script before closing </body> tag.
	html := strings.Replace(buf.String(), "</body>", `<script>window.addEventListener('load',function(){window.print()});</script></body>`, 1)
	w.Write([]byte(html))
}

// ---------- JUnit ----------

func (s *Server) junitService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, _ := s.store.GetService(id)
	if svc == nil {
		http.Error(w, "not found", 404)
		return
	}

	last, _ := s.store.GetLastResult(id)
	if last == nil {
		http.Error(w, "no results", 404)
		return
	}

	// Parse assertion results
	var assertions []junit.AssertionResult
	if last.AssertionResults != "" && last.AssertionResults != "[]" {
		var raw []struct {
			Metric   string  `json:"metric"`
			Operator string  `json:"operator"`
			Value    float64 `json:"value"`
			Actual   float64 `json:"actual"`
			Passed   bool    `json:"passed"`
		}
		_ = json.Unmarshal([]byte(last.AssertionResults), &raw)
		for _, r := range raw {
			assertions = append(assertions, junit.AssertionResult{
				Metric: r.Metric, Operator: r.Operator,
				Value: r.Value, Actual: r.Actual, Passed: r.Passed,
			})
		}
	}

	if len(assertions) == 0 {
		// Create a basic pass/fail based on status
		assertions = append(assertions, junit.AssertionResult{
			Metric: "status", Operator: "eq", Value: 1,
			Actual: func() float64 {
				if last.Status == "pass" {
					return 1
				}
				return 0
			}(),
			Passed: last.Status != "fail",
		})
	}

	dur := time.Duration(last.DurationMs * float64(time.Millisecond))

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-junit.xml\"", svc.Name))
	_ = junit.GenerateFromAssertions(w, svc.Name, dur, assertions)
}

// ---------- share ----------

func (s *Server) shareResult(w http.ResponseWriter, _ *http.Request, serviceID, resultID int64) {
	svc, err := s.store.GetService(serviceID)
	if err != nil || svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	result, err := s.store.GetResult(resultID)
	if err != nil || result == nil {
		http.Error(w, "result not found", http.StatusNotFound)
		return
	}
	if result.ServiceID != serviceID {
		http.Error(w, "result not found", http.StatusNotFound)
		return
	}

	html := generateShareHTML(svc, result)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// shareTimelinePoint mirrors the persisted metrics.TimelinePoint JSON.
type shareTimelinePoint struct {
	T     int64   `json:"t"` // nanoseconds from start
	RPS   float64 `json:"rps"`
	LatMs float64 `json:"lat_ms"`
}

// T2s returns the point's offset in seconds.
func (p shareTimelinePoint) T2s() float64 { return float64(p.T) / 1e9 }

// shareTimelineSVG renders a dark-themed RPS + latency line chart for the share
// page from a persisted timeline JSON string. Returns "" when there's no data.
func shareTimelineSVG(timelineJSON string) string {
	if timelineJSON == "" || timelineJSON == "[]" {
		return ""
	}
	var pts []shareTimelinePoint
	if err := json.Unmarshal([]byte(timelineJSON), &pts); err != nil || len(pts) < 2 {
		return ""
	}

	minT := float64(pts[0].T) / 1e9
	maxT := float64(pts[len(pts)-1].T) / 1e9
	if maxT <= minT {
		maxT = minT + 1
	}

	drawChart := func(title, color, unit string, getVal func(shareTimelinePoint) float64) string {
		const w, h = 640.0, 150.0
		const padL, padR, padT, padB = 48.0, 16.0, 30.0, 24.0
		plotW, plotH := w-padL-padR, h-padT-padB
		maxV := 1.0
		for _, p := range pts {
			if v := getVal(p); v > maxV {
				maxV = v
			}
		}
		maxV *= 1.1
		x := func(t float64) float64 { return padL + (t-minT)/(maxT-minT)*plotW }
		y := func(v float64) float64 { return padT + plotH - v/maxV*plotH }

		var b strings.Builder
		fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto;margin-bottom:12px;">`, w, h)
		fmt.Fprintf(&b, `<rect x="0" y="0" width="%.0f" height="%.0f" rx="8" style="fill:#0f172a;"/>`, w, h)
		fmt.Fprintf(&b, `<text x="%.0f" y="20" style="fill:#f1f5f9;font-size:12px;font-weight:600;font-family:Arial,sans-serif;">%s</text>`, padL, html.EscapeString(title))
		for i := 0; i <= 3; i++ {
			gy := padT + plotH - float64(i)/3.0*plotH
			fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" style="stroke:#1e293b;stroke-width:1;"/>`, padL, gy, padL+plotW, gy)
			fmt.Fprintf(&b, `<text x="%.0f" y="%.1f" style="text-anchor:end;fill:#64748b;font-size:9px;font-family:Arial,sans-serif;">%.0f%s</text>`, padL-6, gy+3, maxV*float64(i)/3.0, unit)
		}
		var line, area strings.Builder
		fmt.Fprintf(&area, "%.1f,%.1f ", x(pts[0].T2s()), padT+plotH)
		for _, p := range pts {
			px, py := x(p.T2s()), y(getVal(p))
			fmt.Fprintf(&line, "%.1f,%.1f ", px, py)
			fmt.Fprintf(&area, "%.1f,%.1f ", px, py)
		}
		fmt.Fprintf(&area, "%.1f,%.1f", x(pts[len(pts)-1].T2s()), padT+plotH)
		fmt.Fprintf(&b, `<polygon points="%s" style="fill:%s;opacity:0.12;"/>`, area.String(), color)
		fmt.Fprintf(&b, `<polyline points="%s" style="fill:none;stroke:%s;stroke-width:2;stroke-linejoin:round;stroke-linecap:round;"/>`, line.String(), color)
		b.WriteString(`</svg>`)
		return b.String()
	}

	rps := drawChart("Requests/sec over time", "#06b6d4", "", func(p shareTimelinePoint) float64 { return p.RPS })
	lat := drawChart("Latency over time", "#8b5cf6", "ms", func(p shareTimelinePoint) float64 { return p.LatMs })
	return rps + lat
}

func generateShareHTML(svc *storage.Service, r *storage.TestResult) string {
	errRate := 0.0
	if r.TotalReqs > 0 {
		errRate = float64(r.Errors) / float64(r.TotalReqs) * 100
	}

	statusColor := "#10b981"
	statusLabel := "PASS"
	if r.Status == "fail" {
		statusColor = "#ef4444"
		statusLabel = "FAIL"
	}

	errColor := "#10b981"
	if errRate > 5 {
		errColor = "#ef4444"
	}

	timelineChart := shareTimelineSVG(r.Timeline)
	timelineSection := ""
	if timelineChart != "" {
		timelineSection = `<div class="card">` + timelineChart + `</div>`
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>gload — %s Test Result</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0f172a; color: #f1f5f9; padding: 2rem; }
        .container { max-width: 700px; margin: 0 auto; }
        .card { background: #1e293b; border: 1px solid rgba(71,85,105,0.3); border-radius: 1rem; padding: 1.5rem; margin-bottom: 1.5rem; }
        .header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; }
        .badge { display: inline-block; padding: 0.2rem 0.6rem; border-radius: 0.375rem; font-size: 0.75rem; font-weight: 700; }
        .grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; }
        .stat { text-align: center; }
        .stat-value { font-size: 1.5rem; font-weight: 700; font-variant-numeric: tabular-nums; }
        .stat-label { font-size: 0.75rem; color: #94a3b8; margin-top: 0.25rem; }
        .meta { font-size: 0.8rem; color: #94a3b8; }
        .footer { text-align: center; margin-top: 2rem; font-size: 0.75rem; color: #64748b; }
        a { color: #7c3aed; }
    </style>
</head>
<body>
    <div class="container">
        <div class="card">
            <div class="header">
                <span style="font-size:1.25rem;font-weight:700;">%s</span>
                <span class="badge" style="background:%s20;color:%s;">%s</span>
                <span class="badge" style="background:rgba(71,85,105,0.3);color:#94a3b8;">%s</span>
            </div>
            <div class="meta">%s</div>
            <div class="meta" style="margin-top:0.25rem;">Tested: %s</div>
        </div>

        <div class="card">
            <div class="grid">
                <div class="stat"><div class="stat-value" style="color:#06b6d4;">%.1f</div><div class="stat-label">Requests/sec</div></div>
                <div class="stat"><div class="stat-value">%d</div><div class="stat-label">Total Requests</div></div>
                <div class="stat"><div class="stat-value" style="color:%s;">%.1f%%</div><div class="stat-label">Error Rate</div></div>
            </div>
        </div>

        <div class="card">
            <h3 style="font-size:0.875rem;font-weight:600;margin-bottom:1rem;">Latency</h3>
            <div class="grid">
                <div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">Avg (ms)</div></div>
                <div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">P95 (ms)</div></div>
                <div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">P99 (ms)</div></div>
                <div class="stat"><div class="stat-value" style="color:#10b981;">%.1f</div><div class="stat-label">Min (ms)</div></div>
                <div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">P50 (ms)</div></div>
                <div class="stat"><div class="stat-value" style="color:#ef4444;">%.1f</div><div class="stat-label">Max (ms)</div></div>
            </div>
        </div>

        %s

        <div class="footer">Generated by <a href="https://github.com/mertgundoganx/gload" target="_blank" rel="noopener">gload</a> — HTTP Load Testing Tool</div>
    </div>
</body>
</html>`,
		html.EscapeString(svc.Name),
		html.EscapeString(svc.Name), statusColor, statusColor, statusLabel, html.EscapeString(svc.Method),
		html.EscapeString(svc.URL),
		r.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
		r.RPS, r.TotalReqs,
		errColor, errRate,
		r.AvgLatencyMs, r.P95LatencyMs, r.P99LatencyMs,
		r.MinLatencyMs, r.P50LatencyMs, r.MaxLatencyMs,
		timelineSection,
	)
}

// ---------- Test Notes ----------

func (s *Server) updateTestNote(w http.ResponseWriter, r *http.Request, resultID int64) {
	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateTestNote(resultID, body.Note); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// deleteResult removes a single test result belonging to the service.
func (s *Server) deleteResult(w http.ResponseWriter, _ *http.Request, serviceID, resultID int64) {
	if err := s.store.DeleteResult(serviceID, resultID); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// deleteCapacityRun removes a single capacity-probe run belonging to the service.
func (s *Server) deleteCapacityRun(w http.ResponseWriter, _ *http.Request, serviceID, runID int64) {
	if err := s.store.DeleteCapacityRun(serviceID, runID); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------- compare report ----------

type compareEntry struct {
	Name   string
	Result *storage.TestResult
}

func (s *Server) handleCompareReport(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Query().Get("ids"), ",")
	if len(parts) < 2 || len(parts) > 5 {
		http.Error(w, "provide 2 to 5 result IDs: ?ids=5,8", http.StatusBadRequest)
		return
	}

	var entries []compareEntry
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			http.Error(w, "invalid IDs", http.StatusBadRequest)
			return
		}
		res, _ := s.store.GetResult(id)
		if res == nil {
			http.Error(w, "result not found", http.StatusNotFound)
			return
		}
		name := "Test"
		if svc, _ := s.store.GetService(res.ServiceID); svc != nil {
			name = svc.Name
		}
		entries = append(entries, compareEntry{Name: name, Result: res})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(generateCompareHTML(entries)))
}

func compareErrRate(r *storage.TestResult) float64 {
	if r.TotalReqs > 0 {
		return float64(r.Errors) / float64(r.TotalReqs) * 100
	}
	return 0
}

// generateCompareHTML renders a clean, light, print-friendly comparative report
// for 2–5 test results. Latency and error rate get a best-value highlight (lower
// is better at comparable load); throughput is not ranked because it depends on
// each run's load settings, which are shown per column.
func generateCompareHTML(entries []compareEntry) string {
	palette := []string{"#7c3aed", "#06b6d4", "#f59e0b", "#059669", "#ef4444"}

	// Per-run config (concurrency/duration/rps) parsed from run_config JSON.
	type runCfg struct {
		Concurrency int    `json:"concurrency"`
		Duration    string `json:"duration"`
		RPS         int    `json:"rps"`
		ArrivalRate int    `json:"arrival_rate"`
	}
	cfgOf := func(r *storage.TestResult) runCfg {
		var c runCfg
		if r.RunConfig != "" {
			_ = json.Unmarshal([]byte(r.RunConfig), &c)
		}
		return c
	}

	// Column headers with per-run metadata.
	var headCells strings.Builder
	for i, e := range entries {
		c := cfgOf(e.Result)
		meta := []string{e.Result.CreatedAt.Format("Jan 2, 15:04")}
		if c.Concurrency > 0 {
			meta = append(meta, fmt.Sprintf("c=%d", c.Concurrency))
		}
		if c.Duration != "" {
			meta = append(meta, html.EscapeString(c.Duration))
		}
		if c.RPS > 0 {
			meta = append(meta, fmt.Sprintf("%d rps cap", c.RPS))
		}
		fmt.Fprintf(&headCells, `<th style="padding:12px 10px;text-align:right;color:%s;vertical-align:bottom;">%s<div style="font-weight:400;font-size:11px;color:#94a3b8;margin-top:2px;">%s</div></th>`,
			palette[i%len(palette)], html.EscapeString(e.Name), strings.Join(meta, " · "))
	}

	// Metric rows. star=true → highlight the best (lowest) value.
	type metric struct {
		label string
		star  bool
		get   func(*storage.TestResult) float64
		fmt   func(float64) string
	}
	ms := func(v float64) string { return fmt.Sprintf("%.1f ms", v) }
	metrics := []metric{
		{"RPS", false, func(r *storage.TestResult) float64 { return r.RPS }, func(v float64) string { return fmt.Sprintf("%.1f /s", v) }},
		{"Total Requests", false, func(r *storage.TestResult) float64 { return float64(r.TotalReqs) }, func(v float64) string { return fmt.Sprintf("%.0f", v) }},
		{"Avg Latency", true, func(r *storage.TestResult) float64 { return r.AvgLatencyMs }, ms},
		{"P50 Latency", true, func(r *storage.TestResult) float64 { return r.P50LatencyMs }, ms},
		{"P95 Latency", true, func(r *storage.TestResult) float64 { return r.P95LatencyMs }, ms},
		{"P99 Latency", true, func(r *storage.TestResult) float64 { return r.P99LatencyMs }, ms},
		{"Max Latency", true, func(r *storage.TestResult) float64 { return r.MaxLatencyMs }, ms},
		{"Error Rate", true, compareErrRate, func(v float64) string { return fmt.Sprintf("%.1f%%", v) }},
		{"Duration", false, func(r *storage.TestResult) float64 { return r.DurationMs / 1000 }, func(v float64) string { return fmt.Sprintf("%.1f s", v) }},
	}

	var rows strings.Builder
	for _, m := range metrics {
		best := math.Inf(1)
		if m.star {
			for _, e := range entries {
				if v := m.get(e.Result); v < best {
					best = v
				}
			}
		}
		fmt.Fprintf(&rows, `<tr style="border-bottom:1px solid #f1f5f9;"><td style="padding:10px;color:#475569;">%s</td>`, m.label)
		for _, e := range entries {
			v := m.get(e.Result)
			style := "color:#0f172a;"
			suffix := ""
			if m.star && v == best {
				style = "color:#059669;font-weight:700;"
				suffix = " ★"
			}
			fmt.Fprintf(&rows, `<td style="padding:10px;text-align:right;font-family:monospace;%s">%s%s</td>`, style, m.fmt(v), suffix)
		}
		rows.WriteString(`</tr>`)
	}

	// Horizontal bar comparison for a single metric across runs.
	bars := func(title string, get func(*storage.TestResult) float64, unit string) string {
		max := 1.0
		for _, e := range entries {
			if v := get(e.Result); v > max {
				max = v
			}
		}
		var b strings.Builder
		fmt.Fprintf(&b, `<div style="font-size:13px;font-weight:600;color:#0f172a;margin-bottom:12px;">%s</div>`, title)
		for i, e := range entries {
			v := get(e.Result)
			pct := v / max * 100
			if pct < 4 {
				pct = 4
			}
			fmt.Fprintf(&b, `<div style="display:flex;align-items:center;gap:12px;margin-bottom:10px;">
				<div style="width:120px;font-size:12px;color:#334155;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">%s</div>
				<div style="flex:1;background:#eef2f7;border-radius:6px;height:26px;position:relative;">
					<div style="width:%.1f%%;height:100%%;border-radius:6px;background:%s;display:flex;align-items:center;justify-content:flex-end;padding-right:8px;min-width:44px;">
						<span style="font-size:11px;font-weight:700;color:#fff;font-family:monospace;">%.1f%s</span>
					</div>
				</div>
			</div>`, html.EscapeString(e.Name), pct, palette[i%len(palette)], v, unit)
		}
		return b.String()
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}

	return fmt.Sprintf(`<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>gload — Comparative Report</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;background:#eef1f6;color:#1e293b;padding:2rem;line-height:1.6}
.container{max-width:960px;margin:0 auto}
.card{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:1.75rem;margin-bottom:1.5rem;box-shadow:0 1px 3px rgba(15,23,42,0.05)}
.hdr{position:relative;overflow:hidden}
.hdr::before{content:'';position:absolute;top:0;left:0;right:0;height:4px;background:linear-gradient(90deg,#6366f1,#8b5cf6,#06b6d4)}
h1{font-size:1.5rem;color:#0f172a;margin-bottom:4px}
table{width:100%%;border-collapse:collapse}
thead th:first-child{text-align:left;color:#64748b;font-size:12px;font-weight:600}
.foot{text-align:center;font-size:12px;color:#94a3b8;margin-top:1rem}
.foot a{color:#7c3aed;text-decoration:none}
.note{font-size:11px;color:#94a3b8;margin-top:12px}
.no-print{text-align:center;margin-bottom:1.25rem}
@media print{.no-print{display:none}body{background:#fff;-webkit-print-color-adjust:exact;print-color-adjust:exact}.card{break-inside:avoid;page-break-inside:avoid}@page{margin:1.2cm}}
</style></head>
<body><div class="container">
<div class="no-print"><button onclick="window.print()" style="background:linear-gradient(135deg,#6366f1,#8b5cf6);color:#fff;border:none;padding:12px 30px;border-radius:10px;font-size:15px;font-weight:700;cursor:pointer;">Save as PDF</button>
<div style="font-size:12px;color:#94a3b8;margin-top:6px;">Opens your browser's print dialog — choose "Save as PDF" as the destination.</div></div>
<div class="card hdr"><h1>Comparative Report</h1><p style="color:#64748b;font-size:14px;">%s &mdash; generated %s</p></div>
<div class="card">%s</div>
<div class="card">%s</div>
<div class="card">
<table>
<thead><tr style="border-bottom:2px solid #e2e8f0;"><th style="padding:12px 10px;">Metric</th>%s</tr></thead>
<tbody>%s</tbody>
</table>
<div class="note">★ = best value (latency &amp; error rate only). Throughput (RPS, total requests) isn't ranked — it depends on each run's load settings shown above, not service quality.</div>
</div>
<div class="foot">Generated by <a href="https://github.com/mertgundoganx/gload" target="_blank" rel="noopener">gload</a> &mdash; HTTP Load Testing Tool</div>
</div></body></html>`,
		html.EscapeString(strings.Join(names, " · ")),
		time.Now().Format("Jan 2, 2006 3:04 PM"),
		bars("Requests / sec", func(r *storage.TestResult) float64 { return r.RPS }, " /s"),
		bars("P95 Latency", func(r *storage.TestResult) float64 { return r.P95LatencyMs }, " ms"),
		headCells.String(),
		rows.String(),
	)
}

func (s *Server) postGitHubComment(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, _ := s.store.GetService(id)
	if svc == nil {
		http.Error(w, "not found", 404)
		return
	}

	last, _ := s.store.GetLastResult(id)
	if last == nil {
		http.Error(w, "no results", 404)
		return
	}

	errRate := 0.0
	if last.TotalReqs > 0 {
		errRate = float64(last.Errors) / float64(last.TotalReqs) * 100
	}

	// Parse assertion results
	var assertions []ghub.AssertionResult
	if last.AssertionResults != "" && last.AssertionResults != "[]" {
		var raw []struct {
			Metric   string  `json:"metric"`
			Operator string  `json:"operator"`
			Value    float64 `json:"value"`
			Actual   float64 `json:"actual"`
			Passed   bool    `json:"passed"`
		}
		_ = json.Unmarshal([]byte(last.AssertionResults), &raw)
		for _, a := range raw {
			assertions = append(assertions, ghub.AssertionResult{
				Metric: a.Metric, Operator: a.Operator,
				Value: a.Value, Actual: a.Actual, Passed: a.Passed,
			})
		}
	}

	result := ghub.TestResult{
		ServiceName: svc.Name,
		Status:      last.Status,
		RPS:         last.RPS,
		AvgLatency:  last.AvgLatencyMs,
		P95Latency:  last.P95LatencyMs,
		P99Latency:  last.P99LatencyMs,
		ErrorRate:   errRate,
		TotalReqs:   last.TotalReqs,
		DurationMs:  last.DurationMs,
		Assertions:  assertions,
	}

	if err := ghub.PostPRComment(result); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "posted"})
}
