package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/metrics"
	"github.com/mertgundoganx/gload/internal/notifier"
	"github.com/mertgundoganx/gload/internal/storage"
	"github.com/mertgundoganx/gload/pkg/config"
)

// dbError logs the real error and returns a generic message to the client.
func dbError(w http.ResponseWriter, err error) {
	logger.Error("database error", logger.Fields("error", err.Error()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// ---------- types ----------

type serviceWithStatus struct {
	storage.Service
	IsRunning   bool           `json:"is_running"`
	RunningKind string         `json:"running_kind,omitempty"` // "capacity" when a probe is running
	LastResult  *snapshotData  `json:"last_result,omitempty"`
	Live        *snapshotData  `json:"live,omitempty"` // live snapshot while a test is running
	Capacity    *capacityBadge `json:"capacity,omitempty"`
}

// capacityBadge is a compact summary of a service's latest capacity probe for
// the dashboard.
type capacityBadge struct {
	MaxRPS          float64 `json:"max_rps"`
	KneeConcurrency int     `json:"knee_concurrency"`
	Reason          string  `json:"reason"`
}

// parseCapacityBadge extracts the dashboard summary from a stored capacity
// result JSON, or nil if it can't be parsed.
func parseCapacityBadge(resultJSON string) *capacityBadge {
	if resultJSON == "" {
		return nil
	}
	var b capacityBadge
	if json.Unmarshal([]byte(resultJSON), &b) != nil || b.MaxRPS <= 0 {
		return nil
	}
	return &b
}

type queueEntry struct {
	ID        int64  `json:"id,omitempty"`
	ServiceID int64  `json:"service_id"`
	Name      string `json:"name"`
}

type profileConfig struct {
	Name        string `json:"name"`
	Concurrency int    `json:"concurrency"`
	Duration    string `json:"duration"`
	RPS         int    `json:"rps"`
}

type loadPattern struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Stages      []patternStage `json:"stages"`
}

type patternStage struct {
	Duration string `json:"duration"`
	Target   int    `json:"target"`
	RPS      int    `json:"rps"`
}

type testTemplate struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Service     templateService `json:"service"`
}

type templateService struct {
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	Concurrency int               `json:"concurrency"`
	Duration    string            `json:"duration"`
	Timeout     string            `json:"timeout"`
	Assertions  string            `json:"assertions"`
	Steps       string            `json:"steps"`
	Validations string            `json:"validations"`
	ThinkTimeMs int               `json:"think_time_ms"`
	ArrivalRate int               `json:"arrival_rate"`
}

type scheduleWithService struct {
	storage.Schedule
	ServiceName string `json:"service_name"`
}

type snapshotData struct {
	DurationMs         float64                `json:"duration_ms"`
	DurationTotalMs    float64                `json:"duration_total_ms"`
	Progress           float64                `json:"progress"`
	TotalReqs          int                    `json:"total_reqs"`
	Errors             int                    `json:"errors"`
	RPS                float64                `json:"rps"`
	AvgLatency         float64                `json:"avg_latency_ms"`
	P50Latency         float64                `json:"p50_latency_ms"`
	P95Latency         float64                `json:"p95_latency_ms"`
	P99Latency         float64                `json:"p99_latency_ms"`
	MinLatency         float64                `json:"min_latency_ms"`
	MaxLatency         float64                `json:"max_latency_ms"`
	StatusCodes        map[int]int            `json:"status_codes"`
	CreatedAt          string                 `json:"created_at,omitempty"`
	Timeline           json.RawMessage        `json:"timeline,omitempty"`
	Status             string                 `json:"status,omitempty"`
	AssertionResults   json.RawMessage        `json:"assertion_results,omitempty"`
	TLSProtocol        string                 `json:"tls_protocol,omitempty"`
	TLSCipherSuite     string                 `json:"tls_cipher_suite,omitempty"`
	TLSServerName      string                 `json:"tls_server_name,omitempty"`
	TLSIssuer          string                 `json:"tls_issuer,omitempty"`
	TLSNotAfter        string                 `json:"tls_not_after,omitempty"`
	TLSHandshakeMs     float64                `json:"tls_handshake_ms,omitempty"`
	RateLimitInfo      *rateLimitData         `json:"rate_limit,omitempty"`
	CircuitState       string                 `json:"circuit_state,omitempty"`
	CircuitEvents      []metrics.CircuitEvent `json:"circuit_events,omitempty"`
	Timing             *timingData            `json:"timing,omitempty"`
	CorrectedReqs      int                    `json:"corrected_reqs,omitempty"`
	ValidationFailures int                    `json:"validation_failures,omitempty"`
	RunConfig          json.RawMessage        `json:"run_config,omitempty"`
}

type timingData struct {
	DNSMs      float64 `json:"dns_ms"`
	TCPMs      float64 `json:"tcp_ms"`
	TLSMs      float64 `json:"tls_ms"`
	TTFBMs     float64 `json:"ttfb_ms"`
	TransferMs float64 `json:"transfer_ms"`
}

type rateLimitData struct {
	Total429s    int                      `json:"total_429s"`
	FirstHitSec  float64                  `json:"first_hit_sec"`
	RetryAfter   string                   `json:"retry_after"`
	Limit        string                   `json:"limit"`
	Remaining    string                   `json:"remaining"`
	Reset        string                   `json:"reset"`
	HitsOverTime []map[string]interface{} `json:"hits_over_time"`
}

// Insight represents a single AI-generated recommendation.
type Insight struct {
	Type     string  `json:"type"`
	Category string  `json:"category"`
	Title    string  `json:"title"`
	Detail   string  `json:"detail"`
	Metric   string  `json:"metric,omitempty"`
	Change   float64 `json:"change,omitempty"`
}

// CapacityProjection represents estimated performance at a given RPS level.
type CapacityProjection struct {
	RPS          float64 `json:"rps"`
	EstLatencyMs float64 `json:"est_latency_ms"`
	EstErrorPct  float64 `json:"est_error_pct"`
	Sustainable  bool    `json:"sustainable"`
}

// CapacityEstimate represents the capacity analysis for a service.
type CapacityEstimate struct {
	CurrentRPS      float64              `json:"current_rps"`
	CurrentP95Ms    float64              `json:"current_p95_ms"`
	CurrentErrorPct float64              `json:"current_error_pct"`
	EstMaxRPS       float64              `json:"est_max_rps"`
	EstSaturationMs float64              `json:"est_saturation_ms"`
	Headroom        float64              `json:"headroom_pct"`
	Projections     []CapacityProjection `json:"projections"`
	Recommendation  string               `json:"recommendation"`
}

// RunConfigData captures the configuration used for a specific test run.
type RunConfigData struct {
	Type        string `json:"type"` // "manual", "profile", "pattern", "queue", "scheduled", "distributed"
	Concurrency int    `json:"concurrency"`
	Duration    string `json:"duration"`
	RPS         int    `json:"rps,omitempty"`
	ArrivalRate int    `json:"arrival_rate,omitempty"`
	ThinkTimeMs int    `json:"think_time_ms,omitempty"`
	PatternName string `json:"pattern_name,omitempty"` // if run via pattern
	ProfileName string `json:"profile_name,omitempty"` // if run via profile
	Stages      string `json:"stages,omitempty"`       // JSON of stages if pattern/stages used
	OpenModel   bool   `json:"open_model,omitempty"`   // stage targets were arrival rate (req/s)
}

// ---------- conversion helpers ----------

func snapshotToRateLimitData(rl *metrics.RateLimitInfo) *rateLimitData {
	if rl == nil || rl.Total429s == 0 {
		return nil
	}
	rd := &rateLimitData{
		Total429s:   rl.Total429s,
		FirstHitSec: rl.FirstHitAt.Seconds(),
		Limit:       rl.RateLimitLimit,
		Remaining:   rl.RateLimitRemaining,
		Reset:       rl.RateLimitReset,
	}
	if rl.RetryAfterSec > 0 {
		rd.RetryAfter = fmt.Sprintf("%.0f", rl.RetryAfterSec)
	}
	for _, p := range rl.HitsOverTime {
		rd.HitsOverTime = append(rd.HitsOverTime, map[string]interface{}{
			"t": p.TimeSec,
			"c": p.Count429,
			"r": p.TotalReq,
		})
	}
	return rd
}

func snapshotJSONWithProgress(s metrics.Snapshot, totalDuration time.Duration) snapshotData {
	progress := 0.0
	totalMs := float64(totalDuration.Milliseconds())
	elapsedMs := float64(s.Duration.Milliseconds())
	if totalMs > 0 {
		progress = elapsedMs / totalMs
		if progress > 1 {
			progress = 1
		}
	}
	sd := snapshotData{
		DurationMs:      elapsedMs,
		DurationTotalMs: totalMs,
		Progress:        progress,
		TotalReqs:       s.TotalReqs,
		Errors:          s.Errors,
		RPS:             s.RPS,
		AvgLatency:      float64(s.AvgLatency.Microseconds()) / 1000,
		P50Latency:      float64(s.P50Latency.Microseconds()) / 1000,
		P95Latency:      float64(s.P95Latency.Microseconds()) / 1000,
		P99Latency:      float64(s.P99Latency.Microseconds()) / 1000,
		MinLatency:      float64(s.MinLatency.Microseconds()) / 1000,
		MaxLatency:      float64(s.MaxLatency.Microseconds()) / 1000,
		StatusCodes:     s.StatusCodes,
	}
	// Include the interval timeline so the live charts can plot true per-interval
	// RPS/latency (not cumulative averages). This path streams ~10x/sec, so cap
	// to the most recent points to bound payload size on long runs — a rolling
	// live window. The full timeline is still persisted for the detail page.
	if n := len(s.Timeline); n > 0 {
		const maxLivePoints = 600 // ~5 min at 500ms sampling
		tl := s.Timeline
		if n > maxLivePoints {
			tl = tl[n-maxLivePoints:]
		}
		if data, err := json.Marshal(tl); err == nil {
			sd.Timeline = data
		}
	}
	if s.TLSInfo != nil {
		sd.TLSProtocol = s.TLSInfo.Protocol
		sd.TLSCipherSuite = s.TLSInfo.CipherSuite
		sd.TLSServerName = s.TLSInfo.ServerName
		sd.TLSIssuer = s.TLSInfo.Issuer
		if !s.TLSInfo.NotAfter.IsZero() {
			sd.TLSNotAfter = s.TLSInfo.NotAfter.Format(time.RFC3339)
		}
		sd.TLSHandshakeMs = float64(s.AvgTLSHandshake.Microseconds()) / 1000
	}
	if s.AvgTiming != nil {
		sd.Timing = &timingData{
			DNSMs:      float64(s.AvgTiming.DNSLookup.Microseconds()) / 1000,
			TCPMs:      float64(s.AvgTiming.TCPConnect.Microseconds()) / 1000,
			TLSMs:      float64(s.AvgTiming.TLSShake.Microseconds()) / 1000,
			TTFBMs:     float64(s.AvgTiming.TTFB.Microseconds()) / 1000,
			TransferMs: float64(s.AvgTiming.Transfer.Microseconds()) / 1000,
		}
	}
	sd.CorrectedReqs = s.CorrectedReqs
	sd.ValidationFailures = s.ValidationFailures
	sd.RateLimitInfo = snapshotToRateLimitData(s.RateLimit)
	if len(s.CircuitEvents) > 0 {
		sd.CircuitEvents = s.CircuitEvents
		sd.CircuitState = s.CircuitEvents[len(s.CircuitEvents)-1].State
	}
	return sd
}

// snapshotToTestResult converts a metrics.Snapshot to a storage.TestResult for persistence.
func snapshotToTestResult(s metrics.Snapshot) storage.TestResult {
	timelineJSON := "[]"
	if len(s.Timeline) > 0 {
		if data, err := json.Marshal(s.Timeline); err == nil {
			timelineJSON = string(data)
		}
	}
	tlsInfoJSON := "{}"
	if s.TLSInfo != nil {
		tlsData := map[string]interface{}{
			"protocol":     s.TLSInfo.Protocol,
			"cipher_suite": s.TLSInfo.CipherSuite,
			"server_name":  s.TLSInfo.ServerName,
			"issuer":       s.TLSInfo.Issuer,
			"not_before":   s.TLSInfo.NotBefore.Format(time.RFC3339),
			"not_after":    s.TLSInfo.NotAfter.Format(time.RFC3339),
			"handshake_ms": float64(s.AvgTLSHandshake.Microseconds()) / 1000,
		}
		if data, err := json.Marshal(tlsData); err == nil {
			tlsInfoJSON = string(data)
		}
	}
	rateLimitJSON := "{}"
	if s.RateLimit != nil && s.RateLimit.Total429s > 0 {
		rd := snapshotToRateLimitData(s.RateLimit)
		if data, err := json.Marshal(rd); err == nil {
			rateLimitJSON = string(data)
		}
	}
	return storage.TestResult{
		DurationMs:    float64(s.Duration.Milliseconds()),
		TotalReqs:     s.TotalReqs,
		Errors:        s.Errors,
		RPS:           s.RPS,
		AvgLatencyMs:  float64(s.AvgLatency.Microseconds()) / 1000,
		P50LatencyMs:  float64(s.P50Latency.Microseconds()) / 1000,
		P95LatencyMs:  float64(s.P95Latency.Microseconds()) / 1000,
		P99LatencyMs:  float64(s.P99Latency.Microseconds()) / 1000,
		MinLatencyMs:  float64(s.MinLatency.Microseconds()) / 1000,
		MaxLatencyMs:  float64(s.MaxLatency.Microseconds()) / 1000,
		StatusCodes:   s.StatusCodes,
		Timeline:      timelineJSON,
		TLSInfo:       tlsInfoJSON,
		RateLimitInfo: rateLimitJSON,
	}
}

// testResultToSnapshot converts a storage.TestResult to a snapshotData for JSON API responses.
func testResultToSnapshot(tr *storage.TestResult) snapshotData {
	var timeline json.RawMessage
	if tr.Timeline != "" && tr.Timeline != "[]" {
		timeline = json.RawMessage(tr.Timeline)
	}
	sd := snapshotData{
		DurationMs:       tr.DurationMs,
		DurationTotalMs:  tr.DurationMs,
		Progress:         1,
		TotalReqs:        tr.TotalReqs,
		Errors:           tr.Errors,
		RPS:              tr.RPS,
		AvgLatency:       tr.AvgLatencyMs,
		P50Latency:       tr.P50LatencyMs,
		P95Latency:       tr.P95LatencyMs,
		P99Latency:       tr.P99LatencyMs,
		MinLatency:       tr.MinLatencyMs,
		MaxLatency:       tr.MaxLatencyMs,
		StatusCodes:      tr.StatusCodes,
		Timeline:         timeline,
		Status:           tr.Status,
		AssertionResults: assertionResultsJSON(tr.AssertionResults),
	}
	if !tr.CreatedAt.IsZero() {
		sd.CreatedAt = tr.CreatedAt.Format(time.RFC3339)
	}

	// Populate TLS fields from stored JSON.
	if tr.TLSInfo != "" && tr.TLSInfo != "{}" {
		var tlsData map[string]interface{}
		if json.Unmarshal([]byte(tr.TLSInfo), &tlsData) == nil {
			if v, ok := tlsData["protocol"].(string); ok {
				sd.TLSProtocol = v
			}
			if v, ok := tlsData["cipher_suite"].(string); ok {
				sd.TLSCipherSuite = v
			}
			if v, ok := tlsData["server_name"].(string); ok {
				sd.TLSServerName = v
			}
			if v, ok := tlsData["issuer"].(string); ok {
				sd.TLSIssuer = v
			}
			if v, ok := tlsData["not_after"].(string); ok {
				sd.TLSNotAfter = v
			}
			if v, ok := tlsData["handshake_ms"].(float64); ok {
				sd.TLSHandshakeMs = v
			}
		}
	}

	// Populate rate limit info from stored JSON.
	if tr.RateLimitInfo != "" && tr.RateLimitInfo != "{}" {
		var rd rateLimitData
		if json.Unmarshal([]byte(tr.RateLimitInfo), &rd) == nil && rd.Total429s > 0 {
			sd.RateLimitInfo = &rd
		}
	}

	// Populate run config from stored JSON.
	if tr.RunConfig != "" && tr.RunConfig != "{}" {
		sd.RunConfig = json.RawMessage(tr.RunConfig)
	}

	return sd
}

func assertionResultsJSON(s string) json.RawMessage {
	if s != "" && s != "[]" {
		return json.RawMessage(s)
	}
	return nil
}

// testResultToSnapshotWithProgress is like testResultToSnapshot but with custom total duration.
func testResultToSnapshotWithProgress(tr *storage.TestResult, totalDuration time.Duration) snapshotData {
	sd := testResultToSnapshot(tr)
	sd.DurationTotalMs = float64(totalDuration.Milliseconds())
	return sd
}

// testResultToMetricsSnapshot converts a storage.TestResult to a metrics.Snapshot for report generation.
func testResultToMetricsSnapshot(tr *storage.TestResult) metrics.Snapshot {
	snap := metrics.Snapshot{
		Duration:    time.Duration(tr.DurationMs * float64(time.Millisecond)),
		TotalReqs:   tr.TotalReqs,
		Errors:      tr.Errors,
		RPS:         tr.RPS,
		AvgLatency:  time.Duration(tr.AvgLatencyMs * float64(time.Millisecond)),
		P50Latency:  time.Duration(tr.P50LatencyMs * float64(time.Millisecond)),
		P95Latency:  time.Duration(tr.P95LatencyMs * float64(time.Millisecond)),
		P99Latency:  time.Duration(tr.P99LatencyMs * float64(time.Millisecond)),
		MinLatency:  time.Duration(tr.MinLatencyMs * float64(time.Millisecond)),
		MaxLatency:  time.Duration(tr.MaxLatencyMs * float64(time.Millisecond)),
		StatusCodes: tr.StatusCodes,
	}
	// Restore the timeline so the report can chart performance over time.
	if tr.Timeline != "" && tr.Timeline != "[]" {
		var tl []metrics.TimelinePoint
		if json.Unmarshal([]byte(tr.Timeline), &tl) == nil {
			snap.Timeline = tl
		}
	}
	return snap
}

// evaluateAssertions checks assertions against a metrics snapshot and returns the status and results JSON.
func evaluateAssertions(snap metrics.Snapshot, assertions []config.Assertion) (string, string) {
	type result struct {
		Metric   string  `json:"metric"`
		Operator string  `json:"operator"`
		Value    float64 `json:"value"`
		Actual   float64 `json:"actual"`
		Passed   bool    `json:"passed"`
	}

	results := make([]result, 0, len(assertions))
	allPassed := true

	for _, a := range assertions {
		var actual float64
		switch a.Metric {
		case "rps":
			actual = snap.RPS
		case "avg_latency":
			actual = float64(snap.AvgLatency.Microseconds()) / 1000
		case "p95_latency":
			actual = float64(snap.P95Latency.Microseconds()) / 1000
		case "p99_latency":
			actual = float64(snap.P99Latency.Microseconds()) / 1000
		case "min_latency":
			actual = float64(snap.MinLatency.Microseconds()) / 1000
		case "max_latency":
			actual = float64(snap.MaxLatency.Microseconds()) / 1000
		case "error_rate":
			if snap.TotalReqs > 0 {
				actual = float64(snap.Errors) / float64(snap.TotalReqs) * 100
			}
		}

		passed := false
		switch a.Operator {
		case "gt":
			passed = actual > a.Value
		case "lt":
			passed = actual < a.Value
		case "gte":
			passed = actual >= a.Value
		case "lte":
			passed = actual <= a.Value
		case "eq":
			passed = actual == a.Value
		}

		if !passed {
			allPassed = false
		}
		results = append(results, result{a.Metric, a.Operator, a.Value, actual, passed})
	}

	status := "pass"
	if !allPassed {
		status = "fail"
	}

	resultsJSON, _ := json.Marshal(results)
	return status, string(resultsJSON)
}

// sendNotification reads notification settings and sends webhook/Slack notifications.
func (s *Server) sendNotification(serviceName, serviceURL string, tr *storage.TestResult) {
	webhookURL, _ := s.store.GetSetting("webhook_url")
	slackURL, _ := s.store.GetSetting("slack_webhook_url")
	teamsURL, _ := s.store.GetSetting("teams_webhook_url")
	discordURL, _ := s.store.GetSetting("discord_webhook_url")
	notifyOn, _ := s.store.GetSetting("notify_on")

	if notifyOn == "none" || notifyOn == "" {
		return
	}
	if notifyOn == "fail_only" && tr.Status != "fail" {
		return
	}

	errRate := 0.0
	if tr.TotalReqs > 0 {
		errRate = float64(tr.Errors) / float64(tr.TotalReqs) * 100
	}

	notifier.Notify(webhookURL, slackURL, teamsURL, discordURL, notifier.TestResult{
		ServiceName: serviceName,
		ServiceURL:  serviceURL,
		Status:      tr.Status,
		TotalReqs:   tr.TotalReqs,
		RPS:         tr.RPS,
		AvgLatency:  tr.AvgLatencyMs,
		P95Latency:  tr.P95LatencyMs,
		P99Latency:  tr.P99LatencyMs,
		ErrorRate:   errRate,
		Duration:    tr.DurationMs,
	})

	// Email
	smtpHost, _ := s.store.GetSetting("smtp_host")
	smtpPort, _ := s.store.GetSetting("smtp_port")
	smtpUser, _ := s.store.GetSetting("smtp_username")
	smtpPass, _ := s.store.GetSetting("smtp_password")
	emailFrom, _ := s.store.GetSetting("email_from")
	emailTo, _ := s.store.GetSetting("email_to")

	if smtpHost != "" && emailTo != "" {
		recipients := strings.Split(emailTo, ",")
		for i := range recipients {
			recipients[i] = strings.TrimSpace(recipients[i])
		}

		emailCfg := notifier.EmailConfig{
			SMTPHost: smtpHost, SMTPPort: smtpPort,
			Username: smtpUser, Password: smtpPass,
			From: emailFrom, To: recipients,
		}
		if err := notifier.SendEmail(emailCfg, notifier.TestResult{
			ServiceName: serviceName, ServiceURL: serviceURL,
			Status: tr.Status, TotalReqs: tr.TotalReqs, RPS: tr.RPS,
			AvgLatency: tr.AvgLatencyMs, P95Latency: tr.P95LatencyMs,
			P99Latency: tr.P99LatencyMs, ErrorRate: errRate, Duration: tr.DurationMs,
		}); err != nil {
			logger.Error("email notification failed", logger.Fields("error", err.Error()))
		}
	}
}
