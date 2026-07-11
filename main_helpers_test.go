package main

import (
	"testing"
	"time"

	"github.com/mertgundoganx/gload/internal/metrics"
)

func TestErrorRate(t *testing.T) {
	if r := errorRate(metrics.Snapshot{TotalReqs: 0, Errors: 0}); r != 0 {
		t.Errorf("zero reqs should give 0, got %v", r)
	}
	if r := errorRate(metrics.Snapshot{TotalReqs: 200, Errors: 10}); r != 5 {
		t.Errorf("10/200 should be 5%%, got %v", r)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		500 * time.Millisecond: "500ms",
		1500 * time.Millisecond: "1.5s",
		30 * time.Second:        "30.0s",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFormatLatency(t *testing.T) {
	if got := formatLatency(0); got != "-" {
		t.Errorf("zero latency = %q, want -", got)
	}
	if got := formatLatency(500 * time.Microsecond); got != "500us" {
		t.Errorf("sub-ms = %q, want 500us", got)
	}
	if got := formatLatency(2500 * time.Microsecond); got != "2.5ms" {
		t.Errorf("2.5ms = %q", got)
	}
}
