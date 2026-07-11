package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/metrics"
)

func renderView(m model) string {
	snap := m.snapshot
	width := 56

	var b strings.Builder

	// Title
	title := titleStyle.Render("  gload")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Target info
	b.WriteString(renderSection("TARGET", width, func(sb *strings.Builder) {
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("URL"),
			valueStyle.Render(m.cfg.URL)))
		sb.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			labelStyle.Width(7).Render("Method"),
			headerStyle.Render(m.cfg.Method),
			labelStyle.Width(12).Render("Concurrency"),
			valueStyle.Render(fmt.Sprintf("%d", m.cfg.Concurrency))))
	}))

	// Progress
	b.WriteString(renderSection("PROGRESS", width, func(sb *strings.Builder) {
		progress := float64(snap.Duration) / float64(m.cfg.Duration)
		if progress > 1 {
			progress = 1
		}
		bar := renderProgressBar(progress, width-12)

		status := successStyle.Render("RUNNING")
		if m.done {
			status = headerStyle.Render("COMPLETED")
		}

		sb.WriteString(fmt.Sprintf("  %s  %s\n", status, mutedStyle.Render(
			fmt.Sprintf("%s / %s", formatDuration(snap.Duration), formatDuration(m.cfg.Duration)))))
		sb.WriteString(fmt.Sprintf("  %s\n", bar))
	}))

	// Metrics
	b.WriteString(renderSection("METRICS", width, func(sb *strings.Builder) {
		errRate := 0.0
		if snap.TotalReqs > 0 {
			errRate = float64(snap.Errors) / float64(snap.TotalReqs) * 100
		}

		sb.WriteString(fmt.Sprintf("  %s %s        %s %s\n",
			labelStyle.Width(10).Render("Requests"),
			valueStyle.Render(formatNumber(snap.TotalReqs)),
			labelStyle.Width(5).Render("RPS"),
			valueStyle.Render(fmt.Sprintf("%.1f", snap.RPS))))

		errRateStr := fmt.Sprintf("%.1f%%", errRate)
		errStyleFn := successStyle
		if errRate > 5 {
			errStyleFn = errorStyle
		} else if errRate > 1 {
			errStyleFn = warningStyle
		}

		sb.WriteString(fmt.Sprintf("  %s %s        %s %s\n",
			labelStyle.Width(10).Render("Success"),
			successStyle.Render(formatNumber(snap.TotalReqs-snap.Errors)),
			labelStyle.Width(5).Render("Err%"),
			errStyleFn.Render(errRateStr)))
	}))

	// Latency
	b.WriteString(renderSection("LATENCY", width, func(sb *strings.Builder) {
		sb.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			labelStyle.Width(7).Render("Avg"),
			valueStyle.Render(formatLatency(snap.AvgLatency)),
			labelStyle.Width(7).Render("Min"),
			mutedStyle.Render(formatLatency(snap.MinLatency))))
		sb.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			labelStyle.Width(7).Render("P50"),
			valueStyle.Render(formatLatency(snap.P50Latency)),
			labelStyle.Width(7).Render("P95"),
			warningStyle.Render(formatLatency(snap.P95Latency))))
		sb.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			labelStyle.Width(7).Render("P99"),
			errorStyle.Render(formatLatency(snap.P99Latency)),
			labelStyle.Width(7).Render("Max"),
			errorStyle.Render(formatLatency(snap.MaxLatency))))
	}))

	// Status codes
	b.WriteString(renderSection("STATUS CODES", width, func(sb *strings.Builder) {
		codes := sortedCodes(snap.StatusCodes)
		for _, code := range codes {
			count := snap.StatusCodes[code]
			pct := 0.0
			if snap.TotalReqs > 0 {
				pct = float64(count) / float64(snap.TotalReqs) * 100
			}

			style := successStyle
			if code >= 500 {
				style = errorStyle
			} else if code >= 400 {
				style = warningStyle
			} else if code == 0 {
				style = errorStyle
			}

			barLen := int(pct / 100 * float64(width-24))
			if barLen < 1 && count > 0 {
				barLen = 1
			}
			bar := style.Render(strings.Repeat("█", barLen))

			label := fmt.Sprintf("%d", code)
			if code == 0 {
				label = "ERR"
			}

			sb.WriteString(fmt.Sprintf("  %s %s %s (%s)\n",
				style.Width(4).Render(label),
				bar,
				mutedStyle.Render(formatNumber(count)),
				mutedStyle.Render(fmt.Sprintf("%.1f%%", pct))))
		}
	}))

	// Latency histogram
	b.WriteString(renderSection("LATENCY DISTRIBUTION", width, func(sb *strings.Builder) {
		sb.WriteString(renderHistogram(snap, width-6))
	}))

	// Footer
	if !m.done {
		b.WriteString(statusBarStyle.Render("  Press q or ctrl+c to stop"))
	} else {
		b.WriteString(statusBarStyle.Render("  Press q or ctrl+c to exit"))
	}
	b.WriteString("\n")

	return b.String()
}

func renderSection(title string, width int, content func(*strings.Builder)) string {
	var sb strings.Builder
	content(&sb)

	header := headerStyle.Render(title)
	inner := sb.String()
	if len(inner) > 0 && inner[len(inner)-1] == '\n' {
		inner = inner[:len(inner)-1]
	}

	box := boxStyle.Width(width).Render(header + "\n" + inner)
	return box + "\n"
}

func renderProgressBar(progress float64, width int) string {
	filled := int(progress * float64(width))
	empty := width - filled

	bar := barFilledStyle.Render(strings.Repeat("█", filled)) +
		barEmptyStyle.Render(strings.Repeat("░", empty))

	pct := fmt.Sprintf(" %.0f%%", progress*100)
	return bar + valueStyle.Render(pct)
}

func renderHistogram(snap metrics.Snapshot, width int) string {
	if len(snap.Latencies) == 0 {
		return "  " + mutedStyle.Render("waiting for data...")
	}

	bucketCount := width
	if bucketCount > 40 {
		bucketCount = 40
	}

	minL := snap.Latencies[0]
	maxL := snap.Latencies[len(snap.Latencies)-1]

	if minL == maxL {
		return fmt.Sprintf("  %s all at %s", mutedStyle.Render("▇"), formatLatency(minL))
	}

	bucketSize := float64(maxL-minL) / float64(bucketCount)
	buckets := make([]int, bucketCount)
	maxBucket := 0

	for _, l := range snap.Latencies {
		idx := int(float64(l-minL) / bucketSize)
		if idx >= bucketCount {
			idx = bucketCount - 1
		}
		buckets[idx]++
		if buckets[idx] > maxBucket {
			maxBucket = buckets[idx]
		}
	}

	bars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var histogram strings.Builder
	histogram.WriteString("  ")

	for _, count := range buckets {
		if count == 0 {
			histogram.WriteString(mutedStyle.Render(" "))
			continue
		}
		height := float64(count) / float64(maxBucket)
		idx := int(height * float64(len(bars)-1))
		style := successStyle
		if height > 0.7 {
			style = warningStyle
		}
		histogram.WriteString(style.Render(bars[idx]))
	}

	histogram.WriteString("\n")
	histogram.WriteString(fmt.Sprintf("  %s%s%s",
		mutedStyle.Render(formatLatency(minL)),
		mutedStyle.Render(strings.Repeat(" ", bucketCount-len(formatLatency(minL))-len(formatLatency(maxL)))),
		mutedStyle.Render(formatLatency(maxL))))

	return histogram.String()
}

func sortedCodes(codes map[int]int) []int {
	keys := make([]int, 0, len(codes))
	for k := range codes {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fus", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}
