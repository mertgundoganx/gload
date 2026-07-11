package metrics

import (
	"testing"
	"time"
)

func TestRecord(t *testing.T) {
	t.Parallel()

	m := New()
	m.Record(200, 10*time.Millisecond, false)
	m.Record(200, 20*time.Millisecond, false)
	m.Record(500, 30*time.Millisecond, true)
	m.Record(404, 40*time.Millisecond, true)

	snap := m.Snapshot()

	if snap.TotalReqs != 4 {
		t.Fatalf("expected 4 total reqs, got %d", snap.TotalReqs)
	}
	if snap.Errors != 2 {
		t.Fatalf("expected 2 errors, got %d", snap.Errors)
	}
	if snap.StatusCodes[200] != 2 {
		t.Fatalf("expected 2 status 200, got %d", snap.StatusCodes[200])
	}
	if snap.StatusCodes[500] != 1 {
		t.Fatalf("expected 1 status 500, got %d", snap.StatusCodes[500])
	}
	if snap.StatusCodes[404] != 1 {
		t.Fatalf("expected 1 status 404, got %d", snap.StatusCodes[404])
	}
}

func TestSnapshot(t *testing.T) {
	t.Parallel()

	m := New()
	latencies := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond, 40 * time.Millisecond, 50 * time.Millisecond}
	for _, l := range latencies {
		m.Record(200, l, false)
	}

	snap := m.Snapshot()

	// Avg should be 30ms
	expectedAvg := 30 * time.Millisecond
	if snap.AvgLatency != expectedAvg {
		t.Fatalf("expected avg latency %v, got %v", expectedAvg, snap.AvgLatency)
	}

	// P50: ceil(0.5*5)-1 = 2 -> sorted[2] = 30ms
	if snap.P50Latency != 30*time.Millisecond {
		t.Fatalf("expected P50 30ms, got %v", snap.P50Latency)
	}

	// P95: ceil(0.95*5)-1 = 4 -> sorted[4] = 50ms
	if snap.P95Latency != 50*time.Millisecond {
		t.Fatalf("expected P95 50ms, got %v", snap.P95Latency)
	}

	// P99: ceil(0.99*5)-1 = 4 -> sorted[4] = 50ms
	if snap.P99Latency != 50*time.Millisecond {
		t.Fatalf("expected P99 50ms, got %v", snap.P99Latency)
	}

	if snap.MinLatency != 10*time.Millisecond {
		t.Fatalf("expected min 10ms, got %v", snap.MinLatency)
	}
	if snap.MaxLatency != 50*time.Millisecond {
		t.Fatalf("expected max 50ms, got %v", snap.MaxLatency)
	}
	if snap.RPS <= 0 {
		t.Fatalf("expected RPS > 0, got %f", snap.RPS)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	t.Parallel()

	m := New()
	snap := m.Snapshot()

	if snap.TotalReqs != 0 {
		t.Fatalf("expected 0 total reqs, got %d", snap.TotalReqs)
	}
	if snap.Errors != 0 {
		t.Fatalf("expected 0 errors, got %d", snap.Errors)
	}
	if snap.RPS != 0 {
		t.Fatalf("expected 0 RPS, got %f", snap.RPS)
	}
	if snap.AvgLatency != 0 {
		t.Fatalf("expected 0 avg latency, got %v", snap.AvgLatency)
	}
	if snap.P50Latency != 0 {
		t.Fatalf("expected 0 P50, got %v", snap.P50Latency)
	}
	if snap.MinLatency != 0 {
		t.Fatalf("expected 0 min, got %v", snap.MinLatency)
	}
	if snap.MaxLatency != 0 {
		t.Fatalf("expected 0 max, got %v", snap.MaxLatency)
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()

	m := New()
	for i := 1; i <= 100; i++ {
		m.Record(200, time.Duration(i)*time.Millisecond, false)
	}

	snap := m.Snapshot()

	// P50: ceil(0.50*100)-1 = 49 -> sorted[49] = 50ms
	if snap.P50Latency != 50*time.Millisecond {
		t.Fatalf("expected P50 ~50ms, got %v", snap.P50Latency)
	}

	// P95: ceil(0.95*100)-1 = 94 -> sorted[94] = 95ms
	if snap.P95Latency != 95*time.Millisecond {
		t.Fatalf("expected P95 ~95ms, got %v", snap.P95Latency)
	}

	// P99: ceil(0.99*100)-1 = 98 -> sorted[98] = 99ms
	if snap.P99Latency != 99*time.Millisecond {
		t.Fatalf("expected P99 ~99ms, got %v", snap.P99Latency)
	}
}

func TestTimeline(t *testing.T) {
	t.Parallel()

	m := New()
	m.Record(200, 10*time.Millisecond, false)
	m.Record(200, 20*time.Millisecond, false)

	m.RecordTimelinePoint()
	time.Sleep(10 * time.Millisecond)
	m.Record(200, 30*time.Millisecond, false)
	m.RecordTimelinePoint()
	time.Sleep(10 * time.Millisecond)
	m.RecordTimelinePoint()

	tl := m.Timeline()
	if len(tl) != 3 {
		t.Fatalf("expected 3 timeline points, got %d", len(tl))
	}

	for i := 1; i < len(tl); i++ {
		if tl[i].Timestamp < tl[i-1].Timestamp {
			t.Fatalf("timeline timestamps not increasing: %v >= %v", tl[i-1].Timestamp, tl[i].Timestamp)
		}
	}
}

func TestSetConcurrency(t *testing.T) {
	t.Parallel()

	m := New()
	m.SetConcurrency(10)
	m.RecordTimelinePoint()

	tl := m.Timeline()
	if len(tl) != 1 {
		t.Fatalf("expected 1 timeline point, got %d", len(tl))
	}
	if tl[0].Concurrency != 10 {
		t.Fatalf("expected concurrency 10, got %d", tl[0].Concurrency)
	}
}

func TestStatusCodes(t *testing.T) {
	t.Parallel()

	m := New()
	m.Record(200, 10*time.Millisecond, false)
	m.Record(200, 10*time.Millisecond, false)
	m.Record(200, 10*time.Millisecond, false)
	m.Record(404, 10*time.Millisecond, true)
	m.Record(404, 10*time.Millisecond, true)
	m.Record(500, 10*time.Millisecond, true)

	snap := m.Snapshot()

	if snap.StatusCodes[200] != 3 {
		t.Fatalf("expected 3x 200, got %d", snap.StatusCodes[200])
	}
	if snap.StatusCodes[404] != 2 {
		t.Fatalf("expected 2x 404, got %d", snap.StatusCodes[404])
	}
	if snap.StatusCodes[500] != 1 {
		t.Fatalf("expected 1x 500, got %d", snap.StatusCodes[500])
	}
}
