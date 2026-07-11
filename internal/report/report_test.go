package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mertgundoganx/gload/internal/metrics"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	snap := metrics.Snapshot{
		Duration:    30 * time.Second,
		TotalReqs:   1000,
		Errors:      5,
		RPS:         33.3,
		AvgLatency:  50 * time.Millisecond,
		P50Latency:  40 * time.Millisecond,
		P95Latency:  120 * time.Millisecond,
		P99Latency:  250 * time.Millisecond,
		MinLatency:  5 * time.Millisecond,
		MaxLatency:  500 * time.Millisecond,
		StatusCodes: map[int]int{200: 990, 500: 10},
	}

	var buf bytes.Buffer
	err := Generate(&buf, snap, "Test API", "https://api.example.com", "POST", 50)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "Test API") {
		t.Error("missing service name")
	}
	if !strings.Contains(html, "https://api.example.com") {
		t.Error("missing URL")
	}
	if !strings.Contains(html, "POST") {
		t.Error("missing method")
	}
	if !strings.Contains(html, "<svg") {
		t.Error("missing SVG charts")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing HTML doctype")
	}
}

func TestGenerateEmptySnapshot(t *testing.T) {
	t.Parallel()

	snap := metrics.Snapshot{
		StatusCodes: map[int]int{},
	}

	var buf bytes.Buffer
	err := Generate(&buf, snap, "Empty", "https://example.com", "GET", 1)
	if err != nil {
		t.Fatalf("Generate failed on empty snapshot: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("empty output")
	}
}

func TestGenerateWithTimeline(t *testing.T) {
	t.Parallel()

	snap := metrics.Snapshot{
		Duration:    10 * time.Second,
		TotalReqs:   100,
		RPS:         10,
		AvgLatency:  50 * time.Millisecond,
		P50Latency:  40 * time.Millisecond,
		P95Latency:  100 * time.Millisecond,
		P99Latency:  200 * time.Millisecond,
		MinLatency:  5 * time.Millisecond,
		MaxLatency:  300 * time.Millisecond,
		StatusCodes: map[int]int{200: 100},
		Timeline: []metrics.TimelinePoint{
			{Timestamp: 1 * time.Second, RPS: 8, AvgLatency: 45},
			{Timestamp: 2 * time.Second, RPS: 12, AvgLatency: 55},
			{Timestamp: 3 * time.Second, RPS: 10, AvgLatency: 50},
		},
	}

	var buf bytes.Buffer
	err := Generate(&buf, snap, "Timeline Test", "https://example.com", "GET", 10)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Should contain timeline SVG charts
	html := buf.String()
	svgCount := strings.Count(html, "<svg")
	if svgCount < 2 {
		t.Errorf("expected at least 2 SVG charts, got %d", svgCount)
	}
}
