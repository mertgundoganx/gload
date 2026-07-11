package server

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/internal/storage"
)

// ---- small formatting helpers ----

func humanInt(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func humanRPS(v float64) string {
	if v >= 100 {
		return humanInt(int64(math.Round(v)))
	}
	return fmt.Sprintf("%.1f", v)
}

func humanLatency(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.2f s", ms/1000)
}

// roundNiceInt rounds a user-count estimate to a readable magnitude.
func roundNiceInt(n float64) int64 {
	switch {
	case n >= 10000:
		return int64(math.Round(n/1000) * 1000)
	case n >= 1000:
		return int64(math.Round(n/100) * 100)
	default:
		return int64(math.Round(n/10) * 10)
	}
}

// capacityCurveSVG plots throughput (y) against each probed level (evenly spaced
// on x), with the knee marked — the "capacity curve" that makes the plateau
// obvious at a glance.
func capacityCurveSVG(res runner.CapacityResult) string {
	steps := res.Steps
	if len(steps) < 2 {
		return ""
	}
	const w, h = 720.0, 300.0
	const padL, padR, padT, padB = 64.0, 24.0, 24.0, 46.0
	plotW, plotH := w-padL-padR, h-padT-padB

	maxRPS := 1.0
	for _, s := range steps {
		if s.RPS > maxRPS {
			maxRPS = s.RPS
		}
	}
	maxY := maxRPS * 1.12
	n := len(steps)
	x := func(i int) float64 {
		if n == 1 {
			return padL + plotW/2
		}
		return padL + float64(i)/float64(n-1)*plotW
	}
	y := func(v float64) float64 { return padT + plotH - v/maxY*plotH }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto;">`, w, h)
	// gridlines + y labels
	for i := 0; i <= 4; i++ {
		gy := padT + plotH - float64(i)/4*plotH
		val := maxY * float64(i) / 4
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e2e8f0" stroke-width="1"/>`, padL, gy, padL+plotW, gy)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.1f" text-anchor="end" fill="#94a3b8" font-size="11" font-family="Arial">%s</text>`, padL-8, gy+4, humanRPS(val))
	}
	// area + line
	var line, area strings.Builder
	fmt.Fprintf(&area, "%.1f,%.1f ", x(0), padT+plotH)
	for i, s := range steps {
		fmt.Fprintf(&line, "%.1f,%.1f ", x(i), y(s.RPS))
		fmt.Fprintf(&area, "%.1f,%.1f ", x(i), y(s.RPS))
	}
	fmt.Fprintf(&area, "%.1f,%.1f", x(n-1), padT+plotH)
	fmt.Fprintf(&b, `<defs><linearGradient id="capgrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="#06b6d4" stop-opacity="0.22"/><stop offset="100%%" stop-color="#06b6d4" stop-opacity="0.02"/></linearGradient></defs>`)
	fmt.Fprintf(&b, `<polygon points="%s" fill="url(#capgrad)"/>`, area.String())
	fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="#06b6d4" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round"/>`, line.String())
	// x labels (concurrency) — thin them out if many
	stepEvery := 1
	if n > 9 {
		stepEvery = (n + 8) / 9
	}
	for i, s := range steps {
		if i%stepEvery != 0 && i != n-1 {
			continue
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="%.0f" text-anchor="middle" fill="#94a3b8" font-size="11" font-family="Arial">%s</text>`, x(i), padT+plotH+18, humanInt(int64(s.Concurrency)))
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.0f" text-anchor="middle" fill="#64748b" font-size="11" font-weight="600" font-family="Arial">Concurrent requests →</text>`, padL+plotW/2, h-6)
	// knee marker
	for i, s := range steps {
		if s.Concurrency == res.KneeConcurrency && res.Reason != "max_reached" {
			kx, ky := x(i), y(s.RPS)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#7c3aed" stroke-width="1" stroke-dasharray="4,4"/>`, kx, ky, kx, padT+plotH)
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#7c3aed" stroke="#fff" stroke-width="2"/>`, kx, ky)
			lx := kx + 8
			anchor := "start"
			if kx > padL+plotW*0.65 {
				lx = kx - 8
				anchor = "end"
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="%s" fill="#7c3aed" font-size="12" font-weight="700" font-family="Arial">knee · %s req/s</text>`, lx, ky-10, anchor, humanRPS(s.RPS))
			break
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// generateCapacityReportHTML renders a clean, print-friendly capacity report
// suitable for sharing with a customer or manager.
func generateCapacityReportHTML(svc *storage.Service, res runner.CapacityResult, createdAt time.Time) string {
	saturated := res.Reason != "max_reached"
	maxRps := res.MaxRPS
	knee := res.KneeConcurrency
	safePer := maxRps * 0.7

	kneeLat := res.SaturationLatencyMs
	for _, s := range res.Steps {
		if s.Concurrency == knee {
			kneeLat = s.AvgLatencyMs
			break
		}
	}

	reasonLabel := map[string]string{
		"throughput_plateau": "throughput stopped growing",
		"latency_degraded":   "latency climbed sharply",
		"errors":             "errors started appearing",
		"max_reached":        "it never saturated within the tested range",
	}[res.Reason]
	if reasonLabel == "" {
		reasonLabel = res.Reason
	}

	headline := fmt.Sprintf("Handled up to ~%s req/sec without saturating", humanRPS(maxRps))
	explanation := ""
	if saturated {
		headline = fmt.Sprintf("Saturates at ~%s concurrent · ~%s req/sec", humanInt(int64(knee)), humanRPS(maxRps))
		explanation = fmt.Sprintf("The system scales smoothly up to about <strong>%s concurrent requests</strong>, sustaining ~<strong>%s requests per second</strong> at ~<strong>%s</strong> response time. Beyond that point %s: adding more load no longer increases throughput — it only increases waiting, with latency rising from %s to %s.",
			humanInt(int64(knee)), humanRPS(maxRps), humanLatency(kneeLat), reasonLabel, humanLatency(res.BaselineLatencyMs), humanLatency(res.SaturationLatencyMs))
	}

	// Real-user estimates.
	userRows := ""
	if maxRps > 0 {
		for _, p := range []struct {
			label string
			gap   int
		}{{"Light — browsing / reading", 20}, {"Typical — active app use", 6}, {"Heavy — very interactive", 2}} {
			userRows += fmt.Sprintf(`<tr><td style="padding:10px 12px;color:#334155;">%s</td><td style="padding:10px 12px;color:#64748b;font-family:monospace;">%ds</td><td style="padding:10px 12px;text-align:right;font-weight:700;color:#059669;font-family:monospace;">~%s</td></tr>`,
				p.label, p.gap, humanInt(roundNiceInt(maxRps*float64(p.gap))))
		}
	}

	// Scaling table: example target loads → instances at 70% headroom.
	instRows := ""
	if safePer > 0 {
		for _, m := range []float64{1, 2, 5, 10} {
			target := maxRps * m
			inst := int64(math.Ceil(target / safePer))
			if inst < 1 {
				inst = 1
			}
			instRows += fmt.Sprintf(`<tr><td style="padding:10px 12px;color:#334155;">%s (%.0f× measured)</td><td style="padding:10px 12px;text-align:right;font-weight:700;color:#0f172a;font-family:monospace;">%s</td></tr>`,
				humanRPS(target)+" req/s", m, plural(inst, "instance"))
		}
	}

	// Per-level table.
	ladder := ""
	maxStepRps := 1.0
	for _, s := range res.Steps {
		if s.RPS > maxStepRps {
			maxStepRps = s.RPS
		}
	}
	for _, s := range res.Steps {
		isKnee := s.Concurrency == knee && saturated
		pct := s.RPS / maxStepRps * 100
		errColor := "#94a3b8"
		if s.ErrorRate > 0.02 {
			errColor = "#dc2626"
		}
		ladder += fmt.Sprintf(`<tr style="%sborder-bottom:1px solid #eef2f7;">
			<td style="padding:9px 12px;font-family:monospace;color:#0f172a;">%s%s</td>
			<td style="padding:9px 12px;"><div style="display:flex;align-items:center;gap:8px;"><div style="flex:1;height:8px;border-radius:4px;background:#eef2f7;"><div style="width:%.0f%%;height:100%%;border-radius:4px;background:#06b6d4;"></div></div><span style="font-family:monospace;font-size:12px;color:#0f172a;width:70px;text-align:right;">%s</span></div></td>
			<td style="padding:9px 12px;text-align:right;font-family:monospace;color:#0f172a;">%s</td>
			<td style="padding:9px 12px;text-align:right;font-family:monospace;color:%s;">%.1f%%</td>
		</tr>`,
			ternary(isKnee, "background:rgba(124,58,237,0.08);", ""),
			humanInt(int64(s.Concurrency)), ternary(isKnee, ` <span style="color:#7c3aed;font-size:11px;">◄ knee</span>`, ""),
			pct, humanRPS(s.RPS), humanLatency(s.AvgLatencyMs), errColor, s.ErrorRate*100)
	}

	curve := capacityCurveSVG(res)

	return fmt.Sprintf(`<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>gload — Capacity Report: %s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;background:#eef1f6;color:#1e293b;padding:2rem;line-height:1.6}
.container{max-width:920px;margin:0 auto}
.card{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:1.75rem;margin-bottom:1.5rem;box-shadow:0 1px 3px rgba(15,23,42,0.05)}
.hdr{position:relative;overflow:hidden}
.hdr::before{content:'';position:absolute;top:0;left:0;right:0;height:4px;background:linear-gradient(90deg,#6366f1,#8b5cf6,#06b6d4)}
h1{font-size:1.4rem;color:#0f172a;margin-bottom:2px}
h2{font-size:1.6rem;color:#0f172a;margin-bottom:6px;line-height:1.3}
h3{font-size:.95rem;color:#0f172a;margin-bottom:.75rem}
.sub{color:#64748b;font-size:.9rem}
.brand{font-weight:800;color:#6d28d9;font-size:1.15rem}
table{width:100%%;border-collapse:collapse;font-size:.9rem}
thead th{text-align:left;color:#64748b;font-size:12px;font-weight:600;padding:8px 12px;border-bottom:2px solid #e2e8f0}
.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:1rem}
.stat{background:#f8fafc;border:1px solid #e2e8f0;border-radius:10px;padding:1rem 1.25rem}
.stat .l{font-size:.7rem;text-transform:uppercase;letter-spacing:.05em;color:#64748b;font-weight:600}
.stat .v{font-size:1.5rem;font-weight:800;color:#0f172a;font-variant-numeric:tabular-nums}
.stat .s{font-size:.72rem;color:#94a3b8;margin-top:2px}
.note{font-size:11px;color:#94a3b8;margin-top:10px}
.foot{text-align:center;font-size:12px;color:#94a3b8;margin-top:1rem}
.foot a{color:#7c3aed;text-decoration:none}
.no-print{text-align:center;margin-bottom:1.25rem}
@media print{.no-print{display:none}body{background:#fff;padding:0}.card{break-inside:avoid;page-break-inside:avoid}@page{margin:1.2cm}}
</style></head>
<body><div class="container">
<div class="no-print"><button onclick="window.print()" style="background:linear-gradient(135deg,#6366f1,#8b5cf6);color:#fff;border:none;padding:12px 30px;border-radius:10px;font-size:15px;font-weight:700;cursor:pointer;">Save as PDF</button>
<div style="font-size:12px;color:#94a3b8;margin-top:6px;">Opens your browser's print dialog — choose "Save as PDF".</div></div>

<div class="card hdr">
  <div style="display:flex;justify-content:space-between;align-items:flex-start;flex-wrap:wrap;gap:10px;">
    <div><span class="brand">⚡ gload</span> <span style="color:#94a3b8;font-size:.8rem;text-transform:uppercase;letter-spacing:.08em;">Capacity Report</span>
      <h1 style="margin-top:8px;">%s</h1>
      <div class="sub" style="font-family:monospace;">%s %s</div>
    </div>
    <div class="sub" style="text-align:right;">%s</div>
  </div>
</div>

<div class="card" style="border-color:rgba(124,58,237,0.3);">
  <h2>%s</h2>
  <p style="color:#475569;">%s</p>
  <div class="stats" style="margin-top:1.25rem;">
    <div class="stat"><div class="l">Sustainable throughput</div><div class="v" style="color:#0891b2;">%s</div><div class="s">requests / second</div></div>
    <div class="stat"><div class="l">Concurrency ceiling</div><div class="v">%s</div><div class="s">simultaneous requests</div></div>
    <div class="stat"><div class="l">Response time at peak</div><div class="v" style="color:#6d28d9;">%s</div><div class="s">vs %s at rest</div></div>
  </div>
</div>

%s

<div class="card">
  <h3>What this means in real users</h3>
  <p class="sub" style="margin-bottom:.9rem;">How many people can use the system at the same time depends on how often each makes a request — a user is mostly idle, so this is far higher than the concurrency number.</p>
  <table><thead><tr><th>User activity</th><th>~1 request every</th><th style="text-align:right;">Concurrent users</th></tr></thead><tbody>%s</tbody></table>
  <div class="note">Based on ~%s req/sec sustained (throughput × seconds between a user's requests).</div>
</div>

<div class="card">
  <h3>Scaling guidance</h3>
  <p style="color:#475569;margin-bottom:.9rem;">One instance sustains ~<strong>%s req/sec</strong>. For headroom, plan around ~<strong>%s req/sec</strong> per instance (70%%). Throughput is capped per instance, so scaling horizontally — more instances behind a load balancer — is the direct lever.</p>
  <table><thead><tr><th>To handle</th><th style="text-align:right;">Instances needed</th></tr></thead><tbody>%s</tbody></table>
</div>

<div class="card">
  <h3>How it was measured</h3>
  <p class="sub" style="margin-bottom:.9rem;">Load was ramped level by level; each level was held and measured at steady state. The knee is the last level before throughput flattened.</p>
  <table><thead><tr><th>Concurrency</th><th>Throughput (req/sec)</th><th style="text-align:right;">Avg latency</th><th style="text-align:right;">Errors</th></tr></thead><tbody>%s</tbody></table>
</div>

<div class="foot">Generated by <a href="https://github.com/mertgundoganx/gload">gload</a> — %s</div>
</div></body></html>`,
		html.EscapeString(svc.Name),
		html.EscapeString(svc.Name),
		html.EscapeString(strings.ToUpper(svc.Method)), html.EscapeString(svc.URL),
		createdAt.Format("Jan 2, 2006 3:04 PM"),
		html.EscapeString(headline),
		explanation,
		humanRPS(maxRps), humanInt(int64(knee)), humanLatency(kneeLat), humanLatency(res.BaselineLatencyMs),
		ternary(curve != "", `<div class="card"><h3>Capacity curve</h3><p class="sub" style="margin-bottom:.5rem;">Throughput rises with load, then flattens at the knee — past that, extra load only adds latency.</p>`+curve+`</div>`, ""),
		userRows,
		humanRPS(maxRps),
		humanRPS(maxRps), humanRPS(safePer),
		instRows,
		ladder,
		createdAt.Format("Jan 2, 2006 3:04 PM"))
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func plural(n int64, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return humanInt(n) + " " + word + "s"
}
