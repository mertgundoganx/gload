package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/metrics"
)

type reportData struct {
	Timestamp   string
	Target      string
	Method      string
	Duration    string
	Concurrency int
	TotalReqs   string
	RPS         string
	Errors      string
	ErrorRate   string
	AvgLatency  string
	P50Latency  string
	P95Latency  string
	P99Latency  string
	MinLatency  string
	MaxLatency  string
	StatusCodes []statusCodeEntry
	LatencyBars []latencyBar

	// SVG charts
	LatencyChart  template.HTML
	DonutChart    template.HTML
	TimelineChart template.HTML
	HasTimeline   bool
	ServiceName   string

	MethodBadgeColor string

	// Pass/fail status
	StatusBadge      string
	StatusBadgeColor string
	StatusBadgeBG    string

	// Executive summary
	ExecSummary string

	// Timing breakdown
	HasTiming    bool
	DNSLookup    string
	TCPConnect   string
	TLSHandshake string
	TTFB         string
	Transfer     string

	// Validation
	ValidationFailures string
	HasValidation      bool

	// Run configuration
	HasRunConfig    bool
	RunType         string // "Manual", "Pattern: Spike Test", "Profile: Light", etc.
	RunConcurrency  string
	RunDuration     string
	RunRPS          string
}

type statusCodeEntry struct {
	Code    string
	Count   string
	Percent string
	Color   string
}

type latencyBar struct {
	Label   string
	Value   string
	Percent float64
}

func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%d\u00b5s", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func statusColor(code int) string {
	switch {
	case code == 0:
		return "#ef4444"
	case code < 300:
		return "#22c55e"
	case code < 400:
		return "#06b6d4"
	case code < 500:
		return "#f59e0b"
	default:
		return "#ef4444"
	}
}

func methodBadgeColor(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "#22c55e"
	case "POST":
		return "#3b82f6"
	case "PUT":
		return "#f59e0b"
	case "DELETE":
		return "#ef4444"
	case "PATCH":
		return "#8b5cf6"
	default:
		return "#6366f1"
	}
}

func latencyMs(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func formatFloat(v float64) string {
	if v < 0.1 {
		return fmt.Sprintf("%.3f", v)
	}
	if v < 10 {
		return fmt.Sprintf("%.2f", v)
	}
	if v < 1000 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.0f", v)
}

func formatPercent(v float64) string {
	if v == 0 {
		return "0%"
	}
	if v < 0.1 {
		return fmt.Sprintf("%.2f%%", v)
	}
	if v >= 100 {
		return "100%"
	}
	return fmt.Sprintf("%.1f%%", v)
}

// generateLatencyBars creates an SVG horizontal bar chart for latency percentiles.
func generateLatencyBars(avg, p50, p95, p99, min, max float64) template.HTML {
	maxVal := max * 1.15
	if maxVal == 0 {
		maxVal = 1
	}

	bars := []struct {
		Label string
		Value float64
		Color string
	}{
		{"Min", min, "#10b981"},
		{"Avg", avg, "#06b6d4"},
		{"P50", p50, "#3b82f6"},
		{"P95", p95, "#f59e0b"},
		{"P99", p99, "#ef4444"},
		{"Max", max, "#dc2626"},
	}

	var svg strings.Builder
	svg.WriteString(`<svg viewBox="0 0 600 210" xmlns="http://www.w3.org/2000/svg" style="width:100%;height:auto;">`)

	barMaxW := 420.0
	labelX := 40.0
	barX := 55.0
	y := 6

	for _, b := range bars {
		pct := b.Value / maxVal * barMaxW
		if pct < 3 && b.Value > 0 {
			pct = 3
		}
		// Label
		svg.WriteString(fmt.Sprintf(
			`<text x="%.0f" y="%d" style="text-anchor:end;fill:#64748b;font-size:12px;font-family:Arial,Helvetica,sans-serif;font-weight:600;">%s</text>`,
			labelX, y+18, b.Label))
		// Bar background
		svg.WriteString(fmt.Sprintf(
			`<rect x="%.0f" y="%d" width="%.0f" height="26" rx="5" style="fill:#eef2f7;"/>`,
			barX, y, barMaxW))
		// Bar fill
		svg.WriteString(fmt.Sprintf(
			`<rect x="%.0f" y="%d" width="%.1f" height="26" rx="5" style="fill:%s;"/>`,
			barX, y, pct, b.Color))
		// Value label - position intelligently: inside bar if wide enough, else to the right
		valStr := formatFloat(b.Value) + "ms"
		textX := barX + pct + 8
		textFill := "#334155"
		if pct > 80 {
			textX = barX + pct - 8
			textFill = "#fff"
			svg.WriteString(fmt.Sprintf(
				`<text x="%.1f" y="%d" style="text-anchor:end;fill:%s;font-size:11px;font-family:Arial,Helvetica,sans-serif;font-weight:700;">%s</text>`,
				textX, y+17, textFill, valStr))
		} else {
			svg.WriteString(fmt.Sprintf(
				`<text x="%.1f" y="%d" style="fill:%s;font-size:11px;font-family:Arial,Helvetica,sans-serif;font-weight:700;">%s</text>`,
				textX, y+17, textFill, valStr))
		}
		y += 33
	}

	svg.WriteString(`</svg>`)
	return template.HTML(svg.String())
}

// generateDonutChart creates an SVG donut chart for status code distribution.
func generateDonutChart(codes map[int]int) template.HTML {
	total := 0
	for _, c := range codes {
		total += c
	}
	if total == 0 {
		return ""
	}

	sortedCodes := make([]int, 0, len(codes))
	for k := range codes {
		sortedCodes = append(sortedCodes, k)
	}
	sort.Ints(sortedCodes)

	var svg strings.Builder
	svg.WriteString(`<svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" style="width:100%;max-width:200px;height:auto;">`)

	cx, cy, r := 100.0, 100.0, 80.0
	innerR := 52.0
	startAngle := -90.0

	for _, code := range sortedCodes {
		count := codes[code]
		if count == 0 {
			continue
		}
		pct := float64(count) / float64(total)
		sweepAngle := pct * 360.0
		color := statusColor(code)

		// A single code at (or near) 100% would produce a degenerate arc whose
		// start and end points coincide, rendering as nothing. Draw a full ring.
		if sweepAngle >= 359.999 {
			midR := (r + innerR) / 2
			svg.WriteString(fmt.Sprintf(
				`<circle cx="%.1f" cy="%.1f" r="%.1f" style="fill:none;stroke:%s;stroke-width:%.1f;"/>`,
				cx, cy, midR, color, r-innerR))
			startAngle += sweepAngle
			continue
		}

		startRad := startAngle * math.Pi / 180
		endRad := (startAngle + sweepAngle) * math.Pi / 180

		x1 := cx + r*math.Cos(startRad)
		y1 := cy + r*math.Sin(startRad)
		x2 := cx + r*math.Cos(endRad)
		y2 := cy + r*math.Sin(endRad)

		ix1 := cx + innerR*math.Cos(startRad)
		iy1 := cy + innerR*math.Sin(startRad)
		ix2 := cx + innerR*math.Cos(endRad)
		iy2 := cy + innerR*math.Sin(endRad)

		largeArc := 0
		if sweepAngle > 180 {
			largeArc = 1
		}

		path := fmt.Sprintf("M %.2f %.2f A %.2f %.2f 0 %d 1 %.2f %.2f L %.2f %.2f A %.2f %.2f 0 %d 0 %.2f %.2f Z",
			x1, y1, r, r, largeArc, x2, y2,
			ix2, iy2, innerR, innerR, largeArc, ix1, iy1)

		svg.WriteString(fmt.Sprintf(
			`<path d="%s" style="fill:%s;stroke:#ffffff;stroke-width:2;"/>`,
			path, color))

		startAngle += sweepAngle
	}

	// Center text
	svg.WriteString(`<text x="100" y="94" style="text-anchor:middle;fill:#64748b;font-size:10px;font-family:Arial,Helvetica,sans-serif;">TOTAL</text>`)
	svg.WriteString(fmt.Sprintf(
		`<text x="100" y="114" style="text-anchor:middle;fill:#0f172a;font-size:18px;font-weight:700;font-family:Arial,Helvetica,sans-serif;">%s</text>`,
		formatNumber(total)))

	svg.WriteString(`</svg>`)
	return template.HTML(svg.String())
}

// generateTimelineSVG creates SVG line charts for RPS and latency over time.
func generateTimelineSVG(timeline []metrics.TimelinePoint) template.HTML {
	if len(timeline) < 2 {
		return ""
	}

	var svg strings.Builder

	chartW := 700.0
	chartH := 160.0
	padL := 60.0
	padR := 20.0
	padT := 35.0
	padB := 30.0
	plotW := chartW - padL - padR
	plotH := chartH - padT - padB

	var maxRPS, maxLat float64
	maxT := timeline[len(timeline)-1].Timestamp.Seconds()
	minT := timeline[0].Timestamp.Seconds()
	if maxT == minT {
		maxT = minT + 1
	}
	for _, p := range timeline {
		if p.RPS > maxRPS {
			maxRPS = p.RPS
		}
		if p.AvgLatency > maxLat {
			maxLat = p.AvgLatency
		}
	}
	if maxRPS == 0 {
		maxRPS = 1
	}
	if maxLat == 0 {
		maxLat = 1
	}
	maxRPS *= 1.1
	maxLat *= 1.1

	drawChart := func(title, color string, yMax float64, getVal func(metrics.TimelinePoint) float64, yUnit string) {
		svg.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto;margin-bottom:16px;">`, chartW, chartH))

		// Background
		svg.WriteString(fmt.Sprintf(
			`<rect x="0" y="0" width="%.0f" height="%.0f" rx="8" style="fill:#f8fafc;stroke:#e2e8f0;stroke-width:1;"/>`, chartW, chartH))

		// Title
		svg.WriteString(fmt.Sprintf(
			`<text x="%.0f" y="22" style="fill:#1e293b;font-size:13px;font-weight:600;font-family:Arial,Helvetica,sans-serif;">%s</text>`,
			padL, title))

		// Grid lines
		for i := 0; i <= 4; i++ {
			gy := padT + plotH - float64(i)/4.0*plotH
			svg.WriteString(fmt.Sprintf(
				`<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" style="stroke:#e2e8f0;stroke-width:1;"/>`,
				padL, gy, padL+plotW, gy))
			label := formatFloat(yMax * float64(i) / 4.0)
			svg.WriteString(fmt.Sprintf(
				`<text x="%.0f" y="%.1f" style="text-anchor:end;fill:#64748b;font-size:9px;font-family:Arial,Helvetica,sans-serif;">%s</text>`,
				padL-6, gy+3, label))
		}

		// Y-axis unit
		svg.WriteString(fmt.Sprintf(
			`<text x="%.0f" y="%.1f" style="text-anchor:end;fill:#64748b;font-size:9px;font-family:Arial,Helvetica,sans-serif;">%s</text>`,
			padL-6, padT-4, yUnit))

		// Build polyline
		var points strings.Builder
		var fillPoints strings.Builder
		for i, p := range timeline {
			x := padL + (p.Timestamp.Seconds()-minT)/(maxT-minT)*plotW
			y := padT + plotH - getVal(p)/yMax*plotH
			if y < padT {
				y = padT
			}
			if i == 0 {
				fillPoints.WriteString(fmt.Sprintf("%.1f,%.1f ", x, padT+plotH))
			}
			points.WriteString(fmt.Sprintf("%.1f,%.1f ", x, y))
			fillPoints.WriteString(fmt.Sprintf("%.1f,%.1f ", x, y))
		}
		lastX := padL + (timeline[len(timeline)-1].Timestamp.Seconds()-minT)/(maxT-minT)*plotW
		fillPoints.WriteString(fmt.Sprintf("%.1f,%.1f", lastX, padT+plotH))

		gradID := strings.ReplaceAll(strings.ToLower(title), " ", "_") + "_grad"
		svg.WriteString(fmt.Sprintf(
			`<defs><linearGradient id="%s" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" style="stop-color:%s;stop-opacity:0.3;"/><stop offset="100%%" style="stop-color:%s;stop-opacity:0.02;"/></linearGradient></defs>`,
			gradID, color, color))

		svg.WriteString(fmt.Sprintf(
			`<polygon points="%s" style="fill:url(#%s);"/>`,
			fillPoints.String(), gradID))

		svg.WriteString(fmt.Sprintf(
			`<polyline points="%s" style="fill:none;stroke:%s;stroke-width:2;stroke-linejoin:round;stroke-linecap:round;"/>`,
			points.String(), color))

		// X-axis labels
		numXLabels := 6
		for i := 0; i <= numXLabels; i++ {
			t := minT + float64(i)/float64(numXLabels)*(maxT-minT)
			x := padL + float64(i)/float64(numXLabels)*plotW
			svg.WriteString(fmt.Sprintf(
				`<text x="%.1f" y="%.0f" style="text-anchor:middle;fill:#64748b;font-size:9px;font-family:Arial,Helvetica,sans-serif;">%.0fs</text>`,
				x, padT+plotH+16, t))
		}

		svg.WriteString(`</svg>`)
	}

	drawChart("Requests per Second", "#06b6d4", maxRPS, func(p metrics.TimelinePoint) float64 { return p.RPS }, "req/s")
	drawChart("Average Latency Over Time", "#8b5cf6", maxLat, func(p metrics.TimelinePoint) float64 { return p.AvgLatency }, "ms")

	return template.HTML(svg.String())
}

// Generate writes a standalone HTML report to w.
func Generate(w io.Writer, snap metrics.Snapshot, serviceName, url, method string, concurrency int, runConfigJSON ...string) error {
	errorRate := 0.0
	if snap.TotalReqs > 0 {
		errorRate = float64(snap.Errors) / float64(snap.TotalReqs) * 100
	}

	// Determine pass/fail status
	statusBadge := "PASS"
	statusBadgeColor := "#22c55e"
	statusBadgeBG := "rgba(34,197,94,0.12)"
	if snap.Errors > 0 {
		if errorRate > 5 {
			statusBadge = "FAIL"
			statusBadgeColor = "#ef4444"
			statusBadgeBG = "rgba(239,68,68,0.12)"
		} else {
			statusBadge = "WARNING"
			statusBadgeColor = "#f59e0b"
			statusBadgeBG = "rgba(245,158,11,0.12)"
		}
	}

	// Status codes
	codes := make([]int, 0, len(snap.StatusCodes))
	for k := range snap.StatusCodes {
		codes = append(codes, k)
	}
	sort.Ints(codes)

	var scEntries []statusCodeEntry
	for _, c := range codes {
		label := fmt.Sprintf("%d", c)
		if c == 0 {
			label = "ERR"
		}
		pct := 0.0
		if snap.TotalReqs > 0 {
			pct = float64(snap.StatusCodes[c]) / float64(snap.TotalReqs) * 100
		}
		scEntries = append(scEntries, statusCodeEntry{
			Code:    label,
			Count:   formatNumber(snap.StatusCodes[c]),
			Percent: formatPercent(pct),
			Color:   statusColor(c),
		})
	}

	// Latency distribution bars
	var bars []latencyBar
	type lEntry struct {
		label string
		val   time.Duration
	}
	entries := []lEntry{
		{"Min", snap.MinLatency},
		{"Avg", snap.AvgLatency},
		{"P50", snap.P50Latency},
		{"P95", snap.P95Latency},
		{"P99", snap.P99Latency},
		{"Max", snap.MaxLatency},
	}
	maxVal := snap.MaxLatency
	if maxVal == 0 {
		maxVal = 1
	}
	for _, e := range entries {
		pct := float64(e.val) / float64(maxVal) * 100
		if pct < 2 && e.val > 0 {
			pct = 2
		}
		bars = append(bars, latencyBar{
			Label:   e.label,
			Value:   formatLatency(e.val),
			Percent: pct,
		})
	}

	// Generate SVG charts
	latencyChart := generateLatencyBars(
		latencyMs(snap.AvgLatency),
		latencyMs(snap.P50Latency),
		latencyMs(snap.P95Latency),
		latencyMs(snap.P99Latency),
		latencyMs(snap.MinLatency),
		latencyMs(snap.MaxLatency),
	)

	donutChart := generateDonutChart(snap.StatusCodes)
	timelineChart := generateTimelineSVG(snap.Timeline)

	displayName := serviceName
	if displayName == "" {
		displayName = url
	}

	// Executive summary
	execSummary := fmt.Sprintf(
		"This test ran for %s with %d concurrent users, achieving %s req/s with an average latency of %s.",
		formatDuration(snap.Duration),
		concurrency,
		fmt.Sprintf("%.1f", snap.RPS),
		formatLatency(snap.AvgLatency),
	)

	// Timing breakdown
	hasTiming := snap.AvgTiming != nil
	var dnsLookup, tcpConnect, tlsHandshake, ttfb, transfer string
	if hasTiming {
		dnsLookup = formatLatency(snap.AvgTiming.DNSLookup)
		tcpConnect = formatLatency(snap.AvgTiming.TCPConnect)
		tlsHandshake = formatLatency(snap.AvgTiming.TLSShake)
		ttfb = formatLatency(snap.AvgTiming.TTFB)
		transfer = formatLatency(snap.AvgTiming.Transfer)
	}

	data := reportData{
		Timestamp:        time.Now().Format("2006-01-02 15:04:05 MST"),
		Target:           url,
		Method:           method,
		Duration:         formatDuration(snap.Duration),
		Concurrency:      concurrency,
		TotalReqs:        formatNumber(snap.TotalReqs),
		RPS:              fmt.Sprintf("%.1f", snap.RPS),
		Errors:           formatNumber(snap.Errors),
		ErrorRate:        formatPercent(errorRate),
		AvgLatency:       formatLatency(snap.AvgLatency),
		P50Latency:       formatLatency(snap.P50Latency),
		P95Latency:       formatLatency(snap.P95Latency),
		P99Latency:       formatLatency(snap.P99Latency),
		MinLatency:       formatLatency(snap.MinLatency),
		MaxLatency:       formatLatency(snap.MaxLatency),
		StatusCodes:      scEntries,
		LatencyBars:      bars,
		LatencyChart:     latencyChart,
		DonutChart:       donutChart,
		TimelineChart:    timelineChart,
		HasTimeline:      len(snap.Timeline) >= 2,
		ServiceName:      displayName,
		MethodBadgeColor: methodBadgeColor(method),
		StatusBadge:      statusBadge,
		StatusBadgeColor: statusBadgeColor,
		StatusBadgeBG:    statusBadgeBG,
		ExecSummary:      execSummary,
		HasTiming:        hasTiming,
		DNSLookup:        dnsLookup,
		TCPConnect:        tcpConnect,
		TLSHandshake:     tlsHandshake,
		TTFB:             ttfb,
		Transfer:         transfer,
		ValidationFailures: formatNumber(snap.ValidationFailures),
		HasValidation:      snap.ValidationFailures > 0,
	}

	// Parse run config if provided
	if len(runConfigJSON) > 0 && runConfigJSON[0] != "" && runConfigJSON[0] != "{}" {
		var rc struct {
			Type        string `json:"type"`
			Concurrency int    `json:"concurrency"`
			Duration    string `json:"duration"`
			RPS         int    `json:"rps"`
			PatternName string `json:"pattern_name"`
			ProfileName string `json:"profile_name"`
			ArrivalRate int    `json:"arrival_rate"`
			Stages      string `json:"stages"`
		}
		if err := json.Unmarshal([]byte(runConfigJSON[0]), &rc); err == nil && rc.Type != "" {
			data.HasRunConfig = true
			switch rc.Type {
			case "pattern":
				data.RunType = "Pattern: " + rc.PatternName
			case "profile":
				data.RunType = "Profile: " + rc.ProfileName
			case "queue":
				data.RunType = "Queue"
			case "scheduled":
				data.RunType = "Scheduled"
			default:
				data.RunType = "Manual"
			}

			// For patterns/stages, show stage summary instead of fixed concurrency
			if rc.Stages != "" && rc.Stages != "[]" {
				var stages []struct {
					Duration string `json:"duration"`
					Target   int    `json:"target"`
					RPS      int    `json:"rps"`
				}
				if err := json.Unmarshal([]byte(rc.Stages), &stages); err == nil && len(stages) > 0 {
					// Find max concurrency and total duration
					maxConc := 0
					totalSec := 0
					var stageFlow []string
					for _, s := range stages {
						if s.Target > maxConc {
							maxConc = s.Target
						}
						stageFlow = append(stageFlow, fmt.Sprintf("%d users", s.Target))
						// Parse duration
						d, err := time.ParseDuration(s.Duration)
						if err == nil {
							totalSec += int(d.Seconds())
						}
					}
					data.Concurrency = maxConc
					data.RunConcurrency = strings.Join(stageFlow, " → ")
					data.Duration = fmt.Sprintf("%ds (staged)", totalSec)
					data.RunDuration = data.Duration
				}
			} else if rc.Concurrency > 0 {
				data.Concurrency = rc.Concurrency
				data.RunConcurrency = formatNumber(rc.Concurrency)
			}

			if rc.Duration != "" && data.RunDuration == "" {
				data.RunDuration = rc.Duration
			}
			if rc.RPS > 0 {
				data.RunRPS = formatNumber(rc.RPS) + " /s"
			} else {
				data.RunRPS = "Unlimited"
			}
		}
	}

	return reportTmpl.Execute(w, data)
}

var reportTmpl = template.Must(template.New("report").Parse(strings.TrimSpace(`
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>gload - Load Test Report</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Arial, Helvetica, sans-serif;
    background: #eef1f6; color: #1e293b; line-height: 1.6;
    padding: 0; margin: 0; min-height: 100vh;
  }
  .page { max-width: 960px; margin: 0 auto; padding: 2rem; }

  /* Header */
  .report-header {
    background: #ffffff;
    border: 1px solid #e2e8f0; border-radius: 16px;
    padding: 2rem 2.5rem; margin-bottom: 1.5rem;
    position: relative; overflow: hidden;
    box-shadow: 0 1px 3px rgba(15,23,42,0.06);
  }
  .report-header::before {
    content: ''; position: absolute; top: 0; left: 0; right: 0; height: 4px;
    background: linear-gradient(90deg, #6366f1, #8b5cf6, #06b6d4);
  }
  .header-top {
    display: flex; align-items: center; justify-content: space-between;
    flex-wrap: wrap; gap: 12px; margin-bottom: 1rem;
  }
  .brand { display: flex; align-items: center; gap: 12px; }
  .brand-logo {
    font-size: 2rem; font-weight: 800; color: #6d28d9;
    letter-spacing: -0.03em;
  }
  .brand-tag {
    font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.12em;
    color: #7c3aed; font-weight: 600; background: rgba(124,58,237,0.1);
    padding: 3px 10px; border-radius: 4px;
  }
  .status-badge {
    display: inline-block; padding: 6px 20px; border-radius: 8px;
    font-weight: 800; font-size: 0.9rem; letter-spacing: 0.06em;
    border: 2px solid;
  }
  .service-name {
    font-size: 1.4rem; font-weight: 700; color: #0f172a; margin-bottom: 0.5rem;
  }
  .header-meta {
    display: flex; flex-wrap: wrap; gap: 12px; align-items: center;
    margin-bottom: 0.75rem;
  }
  .method-badge {
    display: inline-block; padding: 4px 14px; border-radius: 6px;
    font-weight: 700; font-size: 0.85rem; color: #fff;
  }
  .header-url {
    color: #475569; font-size: 0.9rem; word-break: break-all;
    font-family: 'SF Mono', 'Courier New', monospace;
  }
  .header-details {
    display: flex; gap: 2rem; margin-top: 0.5rem; flex-wrap: wrap;
  }
  .header-detail { color: #64748b; font-size: 0.8rem; }
  .header-detail strong { color: #334155; }

  /* Executive summary */
  .exec-summary {
    background: #f5f3ff; border: 1px solid #e2e8f0;
    border-left: 4px solid #8b5cf6;
    border-radius: 10px; padding: 1rem 1.5rem; margin-bottom: 1.5rem;
    color: #334155; font-size: 0.92rem; line-height: 1.7;
  }

  /* Summary cards */
  .summary-grid {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: 1rem; margin-bottom: 1.5rem;
  }
  .summary-card {
    background: #ffffff; border-radius: 12px; padding: 1.25rem 1.5rem;
    border: 1px solid #e2e8f0; text-align: center;
    box-shadow: 0 1px 3px rgba(15,23,42,0.05);
  }
  .summary-label {
    font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.08em;
    color: #64748b; margin-bottom: 0.35rem; font-weight: 600;
  }
  .summary-value {
    font-size: 1.75rem; font-weight: 800; color: #0f172a;
    font-variant-numeric: tabular-nums; line-height: 1.2;
  }
  .summary-sub { font-size: 0.75rem; color: #94a3b8; margin-top: 0.2rem; }
  .text-green { color: #059669; }
  .text-cyan { color: #0891b2; }
  .text-red { color: #dc2626; }
  .text-amber { color: #d97706; }

  /* Sections */
  .section {
    background: #ffffff; border-radius: 12px; padding: 1.75rem;
    border: 1px solid #e2e8f0; margin-bottom: 1.5rem;
    break-inside: avoid;
    box-shadow: 0 1px 3px rgba(15,23,42,0.05);
  }
  .section-title {
    font-size: 0.95rem; font-weight: 700; margin-bottom: 1.25rem;
    color: #0f172a; display: flex; align-items: center; gap: 8px;
  }
  .section-title-dot {
    width: 8px; height: 8px; border-radius: 50%; display: inline-block;
  }

  /* Status code layout */
  .status-layout {
    display: flex; gap: 2.5rem; align-items: center;
  }
  .donut-chart { flex-shrink: 0; }
  .donut-legend { flex: 1; }
  .legend-item {
    display: flex; align-items: center; justify-content: space-between;
    padding: 0.6rem 0; border-bottom: 1px solid #f1f5f9;
  }
  .legend-item:last-child { border-bottom: none; }
  .legend-left { display: flex; align-items: center; gap: 10px; }
  .legend-dot {
    width: 12px; height: 12px; border-radius: 3px; display: inline-block;
  }
  .legend-code { font-weight: 700; font-size: 0.95rem; color: #1e293b; }
  .legend-right { display: flex; align-items: center; gap: 16px; }
  .legend-count { color: #475569; font-size: 0.9rem; font-variant-numeric: tabular-nums; font-weight: 600; }
  .legend-pct { color: #94a3b8; font-size: 0.8rem; font-weight: 600; min-width: 48px; text-align: right; }

  /* Metric grid */
  .metrics-grid {
    display: grid; grid-template-columns: 1fr 1fr; gap: 12px;
  }
  .metric-card {
    background: #f8fafc; border-radius: 10px; padding: 1rem 1.25rem;
    border: 1px solid #e2e8f0;
  }
  .metric-card-label {
    color: #64748b; font-size: 12px; font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.04em;
    margin-bottom: 4px;
  }
  .metric-card-value {
    color: #0f172a; font-size: 18px; font-weight: 700;
    font-variant-numeric: tabular-nums;
  }

  /* Timing breakdown */
  .timing-grid {
    display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px;
  }
  .timing-card {
    background: #f8fafc; border-radius: 10px; padding: 0.85rem 1rem;
    border: 1px solid #e2e8f0; text-align: center;
  }
  .timing-label {
    color: #64748b; font-size: 10px; font-weight: 600;
    text-transform: uppercase; letter-spacing: 0.05em;
    margin-bottom: 4px;
  }
  .timing-value {
    color: #0f172a; font-size: 15px; font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .timing-arrow {
    color: #cbd5e1; font-size: 14px; display: flex;
    align-items: center; justify-content: center;
  }

  /* Footer */
  .report-footer {
    text-align: center; color: #94a3b8; font-size: 0.75rem;
    padding: 1.5rem 0; border-top: 1px solid #e2e8f0;
  }
  .report-footer a { color: #7c3aed; text-decoration: none; }

  /* Print styles — the layout is already a clean light document, so printing
     (and "Save as PDF") reproduces it faithfully. */
  @media print {
    body {
      background: #ffffff;
      -webkit-print-color-adjust: exact !important;
      print-color-adjust: exact !important;
      color-adjust: exact !important;
    }
    @page { margin: 1.2cm; }
    .page { padding: 0; max-width: 100%; }
    .no-print { display: none !important; }
    .section, .summary-card, .report-header, .exec-summary, .timing-grid, .status-layout {
      break-inside: avoid;
      page-break-inside: avoid;
    }
    .summary-grid { break-inside: avoid; page-break-inside: avoid; }
  }

  @media (max-width: 640px) {
    .summary-grid { grid-template-columns: repeat(2, 1fr); }
    .status-layout { flex-direction: column; align-items: flex-start; }
    .metrics-grid { grid-template-columns: 1fr; }
    .timing-grid { grid-template-columns: repeat(3, 1fr); }
  }
</style>
</head>
<body>
<div class="page">

  <!-- Download button -->
  <div class="no-print" style="text-align:center;margin-bottom:20px;">
    <button onclick="window.print()" style="padding:12px 32px;font-size:1rem;font-weight:700;color:#fff;background:linear-gradient(135deg,#6366f1,#8b5cf6);border:none;border-radius:10px;cursor:pointer;box-shadow:0 4px 14px rgba(99,102,241,0.4);letter-spacing:0.02em;">
      Save as PDF
    </button>
    <div style="font-size:0.75rem;color:#94a3b8;margin-top:8px;">Opens your browser's print dialog — choose "Save as PDF" as the destination.</div>
  </div>

  <!-- Header -->
  <div class="report-header">
    <div class="header-top">
      <div class="brand">
        <span class="brand-logo">gload</span>
        <span class="brand-tag">Load Test Report</span>
      </div>
      <span class="status-badge" style="color:{{.StatusBadgeColor}};background:{{.StatusBadgeBG}};border-color:{{.StatusBadgeColor}};">{{.StatusBadge}}</span>
    </div>
    <div class="service-name">{{.ServiceName}}</div>
    <div class="header-meta">
      <span class="method-badge" style="background:{{.MethodBadgeColor}};">{{.Method}}</span>
      <span class="header-url">{{.Target}}</span>
    </div>
    <div class="header-details">
      <span class="header-detail"><strong>Date:</strong> {{.Timestamp}}</span>
      <span class="header-detail"><strong>Duration:</strong> {{.Duration}}</span>
      {{if .HasRunConfig}}
      <span class="header-detail"><strong>Run Type:</strong> {{.RunType}}</span>
      {{if .RunConcurrency}}<span class="header-detail"><strong>Load:</strong> {{.RunConcurrency}}</span>{{end}}
      {{if .RunRPS}}<span class="header-detail"><strong>RPS Limit:</strong> {{.RunRPS}}</span>{{end}}
      {{else}}
      <span class="header-detail"><strong>Concurrency:</strong> {{.Concurrency}}</span>
      {{end}}
    </div>
  </div>

  <!-- Executive Summary -->
  <div class="exec-summary">{{.ExecSummary}}</div>

  <!-- Summary Cards -->
  <div class="summary-grid">
    <div class="summary-card">
      <div class="summary-label">Total Requests</div>
      <div class="summary-value">{{.TotalReqs}}</div>
      <div class="summary-sub">completed</div>
    </div>
    <div class="summary-card">
      <div class="summary-label">Throughput</div>
      <div class="summary-value text-cyan">{{.RPS}}</div>
      <div class="summary-sub">requests/sec</div>
    </div>
    <div class="summary-card">
      <div class="summary-label">Avg Latency</div>
      <div class="summary-value text-green">{{.AvgLatency}}</div>
      <div class="summary-sub">mean response</div>
    </div>
    <div class="summary-card">
      <div class="summary-label">Error Rate</div>
      <div class="summary-value{{if ne .Errors "0"}} text-red{{end}}">{{.ErrorRate}}</div>
      <div class="summary-sub">{{.Errors}} errors</div>
    </div>
  </div>

  <!-- Latency Distribution — FULL WIDTH -->
  <div class="section">
    <div class="section-title">
      <span class="section-title-dot" style="background:#8b5cf6;"></span>
      Latency Percentiles
    </div>
    {{.LatencyChart}}
  </div>

  <!-- Status Code Distribution — FULL WIDTH -->
  <div class="section">
    <div class="section-title">
      <span class="section-title-dot" style="background:#06b6d4;"></span>
      Status Code Distribution
    </div>
    {{if .StatusCodes}}
    <div class="status-layout">
      <div class="donut-chart">{{.DonutChart}}</div>
      <div class="donut-legend">
        {{range .StatusCodes}}
        <div class="legend-item">
          <div class="legend-left">
            <span class="legend-dot" style="background:{{.Color}};"></span>
            <span class="legend-code">{{.Code}}</span>
          </div>
          <div class="legend-right">
            <span class="legend-count">{{.Count}}</span>
            <span class="legend-pct">{{.Percent}}</span>
          </div>
        </div>
        {{end}}
      </div>
    </div>
    {{else}}
    <div style="color:#64748b;text-align:center;padding:2rem 0;">No status code data</div>
    {{end}}
  </div>

  {{if .HasTimeline}}
  <!-- Timeline Charts — FULL WIDTH -->
  <div class="section timeline-section">
    <div class="section-title">
      <span class="section-title-dot" style="background:#22c55e;"></span>
      Performance Over Time
    </div>
    {{.TimelineChart}}
  </div>
  {{end}}

  {{if .HasTiming}}
  <!-- Timing Breakdown -->
  <div class="section">
    <div class="section-title">
      <span class="section-title-dot" style="background:#06b6d4;"></span>
      Request Timing Breakdown (Average)
    </div>
    <div class="timing-grid">
      <div class="timing-card">
        <div class="timing-label">DNS Lookup</div>
        <div class="timing-value">{{.DNSLookup}}</div>
      </div>
      <div class="timing-card">
        <div class="timing-label">TCP Connect</div>
        <div class="timing-value">{{.TCPConnect}}</div>
      </div>
      <div class="timing-card">
        <div class="timing-label">TLS Handshake</div>
        <div class="timing-value">{{.TLSHandshake}}</div>
      </div>
      <div class="timing-card">
        <div class="timing-label">Time to First Byte</div>
        <div class="timing-value">{{.TTFB}}</div>
      </div>
      <div class="timing-card">
        <div class="timing-label">Content Transfer</div>
        <div class="timing-value">{{.Transfer}}</div>
      </div>
    </div>
  </div>
  {{end}}

  <!-- Detailed Metrics — FULL WIDTH grid -->
  <div class="section">
    <div class="section-title">
      <span class="section-title-dot" style="background:#f59e0b;"></span>
      Detailed Metrics
    </div>
    <div class="metrics-grid">
      <div class="metric-card">
        <div class="metric-card-label">Total Requests</div>
        <div class="metric-card-value">{{.TotalReqs}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Requests / Second</div>
        <div class="metric-card-value">{{.RPS}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Average Latency</div>
        <div class="metric-card-value">{{.AvgLatency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">P50 Latency</div>
        <div class="metric-card-value">{{.P50Latency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">P95 Latency</div>
        <div class="metric-card-value">{{.P95Latency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">P99 Latency</div>
        <div class="metric-card-value">{{.P99Latency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Min Latency</div>
        <div class="metric-card-value">{{.MinLatency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Max Latency</div>
        <div class="metric-card-value">{{.MaxLatency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Errors</div>
        <div class="metric-card-value">{{.Errors}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Error Rate</div>
        <div class="metric-card-value">{{.ErrorRate}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Concurrency</div>
        <div class="metric-card-value">{{.Concurrency}}</div>
      </div>
      <div class="metric-card">
        <div class="metric-card-label">Duration</div>
        <div class="metric-card-value">{{.Duration}}</div>
      </div>
      {{if .HasValidation}}
      <div class="metric-card">
        <div class="metric-card-label">Validation Failures</div>
        <div class="metric-card-value" style="color:#ef4444;">{{.ValidationFailures}}</div>
      </div>
      {{end}}
    </div>
  </div>

  <!-- Footer -->
  <div class="report-footer">
    Generated by <a href="https://github.com/mertgundoganx/gload"><strong style="color:#8b5cf6;">gload</strong></a> &mdash; {{.Timestamp}}
  </div>

</div>
</body>
</html>
`)))
