package server

import (
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/mertgundoganx/gload/internal/storage"
)

func (s *Server) getInsights(w http.ResponseWriter, r *http.Request, id int64) {
	results, err := s.store.ListResults(id, 10)
	if err != nil || len(results) < 2 {
		writeJSON(w, http.StatusOK, []Insight{})
		return
	}

	insights := analyzeResults(results)
	writeJSON(w, http.StatusOK, insights)
}

var insightSeverityOrder = map[string]int{
	"critical": 0,
	"warning":  1,
	"info":     2,
	"success":  3,
}

func analyzeResults(results []storage.TestResult) []Insight {
	var insights []Insight

	// Results are ordered newest first from storage.
	latest := results[0]

	// 1 & 2: Latency trending (compare last 3 vs previous 3)
	if len(results) >= 6 {
		var recentP95, prevP95 float64
		for i := 0; i < 3; i++ {
			recentP95 += results[i].P95LatencyMs
			prevP95 += results[i+3].P95LatencyMs
		}
		recentP95 /= 3
		prevP95 /= 3
		if prevP95 > 0 {
			change := (recentP95 - prevP95) / prevP95 * 100
			if change > 20 {
				insights = append(insights, Insight{
					Type:     "warning",
					Category: "latency",
					Title:    "P95 latency trending up",
					Detail:   fmt.Sprintf("P95 latency increased %.0f%% over last 3 tests (was %.0fms, now %.0fms). Consider scaling your service.", change, prevP95, recentP95),
					Metric:   "p95_latency_ms",
					Change:   change,
				})
			} else if change < -15 {
				insights = append(insights, Insight{
					Type:     "success",
					Category: "latency",
					Title:    "P95 latency improving",
					Detail:   fmt.Sprintf("P95 latency improved %.0f%% — your recent optimizations are working.", -change),
					Metric:   "p95_latency_ms",
					Change:   change,
				})
			}
		}
	}

	// 3 & 4: RPS trending
	if len(results) >= 6 {
		var recentRPS, prevRPS float64
		for i := 0; i < 3; i++ {
			recentRPS += results[i].RPS
			prevRPS += results[i+3].RPS
		}
		recentRPS /= 3
		prevRPS /= 3
		if prevRPS > 0 {
			change := (recentRPS - prevRPS) / prevRPS * 100
			if change < -15 {
				insights = append(insights, Insight{
					Type:     "warning",
					Category: "throughput",
					Title:    "Throughput declining",
					Detail:   fmt.Sprintf("Throughput decreased %.0f%% over recent tests. Check for resource contention.", -change),
					Metric:   "rps",
					Change:   change,
				})
			} else if change > 15 {
				insights = append(insights, Insight{
					Type:     "success",
					Category: "throughput",
					Title:    "Throughput improving",
					Detail:   fmt.Sprintf("Throughput improved %.0f%% — service is handling more load.", change),
					Metric:   "rps",
					Change:   change,
				})
			}
		}
	}

	// 5: Error rate spike
	if len(results) >= 2 {
		latestErrorRate := 0.0
		if latest.TotalReqs > 0 {
			latestErrorRate = float64(latest.Errors) / float64(latest.TotalReqs) * 100
		}
		var prevSum float64
		prevCount := 0
		for i := 1; i < len(results); i++ {
			if results[i].TotalReqs > 0 {
				prevSum += float64(results[i].Errors) / float64(results[i].TotalReqs) * 100
				prevCount++
			}
		}
		if prevCount > 0 {
			prevAvg := prevSum / float64(prevCount)
			if prevAvg > 0 && latestErrorRate > 2*prevAvg {
				insights = append(insights, Insight{
					Type:     "critical",
					Category: "errors",
					Title:    "Error rate spike detected",
					Detail:   fmt.Sprintf("Error rate spiked to %.1f%% (average was %.1f%%). Investigate recent deployments.", latestErrorRate, prevAvg),
					Metric:   "error_rate",
					Change:   latestErrorRate,
				})
			}
		}

		// 6: High error rate
		if latestErrorRate > 5 {
			insights = append(insights, Insight{
				Type:     "critical",
				Category: "errors",
				Title:    "High error rate",
				Detail:   fmt.Sprintf("Error rate is %.1f%% — significantly above acceptable threshold.", latestErrorRate),
				Metric:   "error_rate",
			})
		}
	}

	// 7: Stability (RPS standard deviation)
	if len(results) >= 3 {
		var sum float64
		for _, r := range results {
			sum += r.RPS
		}
		mean := sum / float64(len(results))
		if mean > 0 {
			var variance float64
			for _, r := range results {
				d := r.RPS - mean
				variance += d * d
			}
			stddev := math.Sqrt(variance / float64(len(results)))
			cv := stddev / mean
			if cv < 0.10 {
				insights = append(insights, Insight{
					Type:     "success",
					Category: "stability",
					Title:    "Stable performance",
					Detail:   "Service shows stable performance across test runs.",
					Metric:   "rps",
				})
			} else {
				insights = append(insights, Insight{
					Type:     "warning",
					Category: "stability",
					Title:    "Performance variance detected",
					Detail:   fmt.Sprintf("Performance varies significantly between tests (RPS σ=%.1f). Check for external dependencies.", stddev),
					Metric:   "rps",
				})
			}
		}
	}

	// 8: Capacity warning
	if latest.P95LatencyMs > 500 {
		insights = append(insights, Insight{
			Type:     "warning",
			Category: "capacity",
			Title:    "Approaching capacity limits",
			Detail:   fmt.Sprintf("P95 latency of %.0fms suggests the service is approaching capacity limits.", latest.P95LatencyMs),
			Metric:   "p95_latency_ms",
		})
	}

	// 9: Consecutive failures
	failCount := 0
	for _, r := range results {
		if r.Status == "fail" {
			failCount++
		} else {
			break
		}
	}
	if failCount >= 2 {
		insights = append(insights, Insight{
			Type:     "critical",
			Category: "errors",
			Title:    "Consecutive test failures",
			Detail:   fmt.Sprintf("Last %d tests failed assertions. Service may need immediate attention.", failCount),
		})
	}

	// 10: Recovery detected
	if len(results) >= 2 && latest.Status == "pass" && results[1].Status == "fail" {
		insights = append(insights, Insight{
			Type:     "info",
			Category: "stability",
			Title:    "Service recovered",
			Detail:   "Service recovered — latest test passed after previous failure.",
		})
	}

	// Sort by severity: critical, warning, info, success
	sort.Slice(insights, func(i, j int) bool {
		return insightSeverityOrder[insights[i].Type] < insightSeverityOrder[insights[j].Type]
	})

	return insights
}

// ---------- Analytics: Capacity ----------

func (s *Server) getCapacity(w http.ResponseWriter, r *http.Request, id int64) {
	results, err := s.store.ListResults(id, 10)
	if err != nil || len(results) == 0 {
		http.Error(w, "no test results available", http.StatusNotFound)
		return
	}

	svc, err := s.store.GetService(id)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	latest := results[0]
	currentErrorPct := 0.0
	if latest.TotalReqs > 0 {
		currentErrorPct = float64(latest.Errors) / float64(latest.TotalReqs) * 100
	}

	// Use Little's Law: maxRPS ≈ concurrency / (latency_in_seconds)
	concurrency := float64(svc.Concurrency)
	if concurrency < 1 {
		concurrency = 1
	}
	latencySec := latest.P95LatencyMs / 1000.0
	if latencySec <= 0 {
		latencySec = 0.001
	}
	estMaxRPS := concurrency / latencySec

	// Estimate saturation latency (at max RPS)
	k := 0.5
	estSaturationMs := latest.P95LatencyMs * math.Exp(k*(estMaxRPS/latest.RPS-1))
	if estSaturationMs > 10000 {
		estSaturationMs = 10000
	}

	// Headroom
	headroom := 0.0
	if estMaxRPS > 0 && latest.RPS > 0 {
		headroom = (estMaxRPS - latest.RPS) / estMaxRPS * 100
		if headroom < 0 {
			headroom = 0
		}
	}

	// Projections at 1x, 1.5x, 2x, 3x, 5x current RPS
	multipliers := []float64{1, 1.5, 2, 3, 5}
	var projections []CapacityProjection
	for _, m := range multipliers {
		targetRPS := latest.RPS * m
		estLatency := latest.P95LatencyMs * math.Exp(k*(targetRPS/latest.RPS-1))
		estErrPct := currentErrorPct * math.Exp(k*(targetRPS/latest.RPS-1))
		sustainable := estLatency < 1000 && estErrPct < 10
		projections = append(projections, CapacityProjection{
			RPS:          math.Round(targetRPS*100) / 100,
			EstLatencyMs: math.Round(estLatency*100) / 100,
			EstErrorPct:  math.Round(estErrPct*100) / 100,
			Sustainable:  sustainable,
		})
	}

	recommendation := ""
	if headroom > 50 {
		recommendation = fmt.Sprintf("Service has %.0f%% headroom. Current capacity is sufficient for growth.", headroom)
	} else if headroom > 20 {
		recommendation = fmt.Sprintf("Service has %.0f%% headroom. Plan for scaling if traffic increases beyond %.0f RPS.", headroom, estMaxRPS)
	} else {
		recommendation = fmt.Sprintf("Service is near capacity (%.0f%% headroom). Consider scaling immediately.", headroom)
	}

	estimate := CapacityEstimate{
		CurrentRPS:      math.Round(latest.RPS*100) / 100,
		CurrentP95Ms:    math.Round(latest.P95LatencyMs*100) / 100,
		CurrentErrorPct: math.Round(currentErrorPct*100) / 100,
		EstMaxRPS:       math.Round(estMaxRPS*100) / 100,
		EstSaturationMs: math.Round(estSaturationMs*100) / 100,
		Headroom:        math.Round(headroom*100) / 100,
		Projections:     projections,
		Recommendation:  recommendation,
	}

	writeJSON(w, http.StatusOK, estimate)
}
