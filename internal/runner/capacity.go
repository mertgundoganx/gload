package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CapacityStep is one probed concurrency level and the steady-state behaviour
// observed at it.
type CapacityStep struct {
	Concurrency  int     `json:"concurrency"`
	RPS          float64 `json:"rps"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

// CapacityResult is the outcome of a capacity probe: the per-level measurements
// plus the detected "knee" (the point of diminishing returns / saturation).
type CapacityResult struct {
	Steps               []CapacityStep `json:"steps"`
	KneeConcurrency     int            `json:"knee_concurrency"`      // best sustainable concurrency (0 = never saturated)
	MaxRPS              float64        `json:"max_rps"`               // best sustained throughput observed
	BaselineLatencyMs   float64        `json:"baseline_latency_ms"`   // latency at low load
	SaturationLatencyMs float64        `json:"saturation_latency_ms"` // latency at the saturating level
	Reason              string         `json:"reason"`                // throughput_plateau | latency_degraded | errors | max_reached
}

// CapacityConfig tunes the probe. Zero values fall back to sensible defaults.
type CapacityConfig struct {
	Start        int           // starting concurrency (default 5)
	Factor       float64       // multiply concurrency each step (default 1.6)
	Max          int           // stop climbing past this (default 2000)
	StepRamp     time.Duration // time to reach + stabilize a level (default 3s)
	StepHold     time.Duration // measurement window per level (default 10s)
	ErrThreshold float64       // error rate that means saturation (default 0.05)
	LatDegrade   float64       // avg latency multiple over baseline that means saturation (default 4.0)
	MinRPSGain   float64       // min fractional RPS gain to consider a level "still scaling" (default 0.10)
}

func (c *CapacityConfig) applyDefaults() {
	if c.Start <= 0 {
		c.Start = 5
	}
	if c.Factor < 1.1 {
		c.Factor = 1.6
	}
	if c.Max <= 0 {
		// A high safety ceiling, not a target: the probe stops at the knee
		// (throughput plateau) well before this for any real system — and if a
		// target has enormous capacity, the load generator itself plateaus
		// first. So the user never has to "raise the max".
		c.Max = 10000
	}
	if c.StepRamp <= 0 {
		c.StepRamp = 3 * time.Second
	}
	if c.StepHold <= 0 {
		c.StepHold = 10 * time.Second
	}
	if c.ErrThreshold <= 0 {
		c.ErrThreshold = 0.05
	}
	if c.LatDegrade <= 0 {
		c.LatDegrade = 4.0
	}
	if c.MinRPSGain <= 0 {
		c.MinRPSGain = 0.10
	}
}

// buildLevels produces the increasing concurrency steps to probe.
func (c CapacityConfig) buildLevels() []int {
	var levels []int
	cur := float64(c.Start)
	last := 0
	for int(cur) <= c.Max {
		l := int(cur)
		if l > last { // avoid duplicates from rounding
			levels = append(levels, l)
			last = l
		}
		cur *= c.Factor
	}
	if last < c.Max {
		levels = append(levels, c.Max)
	}
	return levels
}

// RunCapacityProbe drives load up one concurrency level at a time, measures the
// steady-state throughput/latency/errors at each, and stops as soon as it
// detects the knee — the point where adding load stops buying throughput and
// starts buying latency (or errors). This automates "how much can my system
// take before it degrades?" without overshooting into a pointless overload.
func (r *Runner) RunCapacityProbe(ctx context.Context, cc CapacityConfig) CapacityResult {
	cc.applyDefaults()

	var (
		desired atomic.Int64
		wg      sync.WaitGroup
		spawned int
	)

	worker := func(id int64) {
		defer wg.Done()
		wc := r.newWorkerCtx(int(id))
		for {
			if ctx.Err() != nil {
				return
			}
			if id >= desired.Load() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
				continue
			}
			r.sendRequestWithClient(ctx, r.client, wc)
		}
	}
	ensureWorkers := func(n int) {
		for spawned < n {
			wg.Add(1)
			go worker(int64(spawned))
			spawned++
		}
	}

	// Sample the cumulative counters for windowed diffs.
	type sample struct {
		reqs, errs, latSum, latCount int64
		at                           time.Time
	}
	take := func() sample {
		reqs, errs, latSum, latCount := r.Metrics.Totals()
		return sample{reqs, errs, latSum, latCount, time.Now()}
	}

	var result CapacityResult
	bestRPS := 0.0
	prevGood := 0

	for _, level := range cc.buildLevels() {
		if ctx.Err() != nil {
			break
		}
		ensureWorkers(level)
		desired.Store(int64(level))
		r.Metrics.SetConcurrency(level)

		// Let the level reach steady state before measuring.
		if !sleepCtx(ctx, cc.StepRamp) {
			break
		}
		s0 := take()
		if !sleepCtx(ctx, cc.StepHold) {
			break
		}
		s1 := take()

		dt := s1.at.Sub(s0.at).Seconds()
		reqs := s1.reqs - s0.reqs
		if dt <= 0 || reqs <= 0 {
			continue
		}
		rps := float64(reqs) / dt
		errRate := float64(s1.errs-s0.errs) / float64(reqs)
		avgLat := 0.0
		if lc := s1.latCount - s0.latCount; lc > 0 {
			avgLat = float64(s1.latSum-s0.latSum) / float64(lc) / 1e6
		}

		result.Steps = append(result.Steps, CapacityStep{
			Concurrency: level, RPS: rps, AvgLatencyMs: avgLat, ErrorRate: errRate,
		})
		if result.BaselineLatencyMs == 0 && avgLat > 0 {
			result.BaselineLatencyMs = avgLat
		}

		// Knee detection, in priority order.
		reason := ""
		switch {
		case errRate > cc.ErrThreshold:
			reason = "errors"
		case result.BaselineLatencyMs > 0 && avgLat > result.BaselineLatencyMs*cc.LatDegrade:
			reason = "latency_degraded"
		case bestRPS > 0 && rps < bestRPS*(1+cc.MinRPSGain):
			reason = "throughput_plateau"
		}
		if rps > bestRPS {
			bestRPS = rps
		}

		if reason != "" {
			result.Reason = reason
			result.SaturationLatencyMs = avgLat
			result.KneeConcurrency = prevGood
			if result.KneeConcurrency == 0 {
				result.KneeConcurrency = level
			}
			break
		}
		prevGood = level
	}

	if result.Reason == "" {
		// Climbed to Max without saturating.
		result.Reason = "max_reached"
		result.KneeConcurrency = prevGood
	}
	result.MaxRPS = bestRPS

	desired.Store(0)
	// Best-effort drain; ctx is usually still live here.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	return result
}

// sleepCtx sleeps for d, returning false if ctx was cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
