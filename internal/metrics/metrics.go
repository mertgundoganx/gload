package metrics

import (
	"math"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// latencyHistogram is a memory-efficient data structure for tracking latency
// distributions. Instead of storing every latency value (which can consume
// hundreds of MB for long tests), it uses bucketed counting with variable
// precision: 1us for <10ms, 10us for 10-100ms, 100us for 100ms-1s, 1ms for >1s.
// This keeps memory usage under ~500KB regardless of test duration.
type latencyHistogram struct {
	counts map[int64]int64 // microsecond bucket -> count
	count  int64
	sum    int64 // total nanoseconds
	min    int64 // nanoseconds, initialized to MaxInt64
	max    int64 // nanoseconds

	// Cached sorted buckets for fast percentile queries during live streaming.
	sortedCache      []int64
	sortedCacheDirty bool
}

func newLatencyHistogram() *latencyHistogram {
	return &latencyHistogram{
		counts: make(map[int64]int64, 256),
		min:    math.MaxInt64,
	}
}

func (h *latencyHistogram) record(d time.Duration) {
	ns := d.Nanoseconds()
	h.count++
	h.sum += ns
	if ns < h.min {
		h.min = ns
	}
	if ns > h.max {
		h.max = ns
	}

	// Bucket by microsecond with precision reduction for larger values.
	us := d.Microseconds()
	var bucket int64
	switch {
	case us < 10000: // <10ms: 1us precision
		bucket = us
	case us < 100000: // 10-100ms: 10us precision
		bucket = (us / 10) * 10
	case us < 1000000: // 100ms-1s: 100us precision
		bucket = (us / 100) * 100
	default: // >1s: 1ms precision
		bucket = (us / 1000) * 1000
	}
	h.counts[bucket]++
	h.sortedCacheDirty = true
}

func (h *latencyHistogram) percentile(p float64) time.Duration {
	if h.count == 0 {
		return 0
	}

	// Rebuild sorted cache if dirty.
	if h.sortedCacheDirty || h.sortedCache == nil {
		h.sortedCache = make([]int64, 0, len(h.counts))
		for b := range h.counts {
			h.sortedCache = append(h.sortedCache, b)
		}
		sort.Slice(h.sortedCache, func(i, j int) bool { return h.sortedCache[i] < h.sortedCache[j] })
		h.sortedCacheDirty = false
	}

	target := int64(math.Ceil(p / 100 * float64(h.count)))
	var cumulative int64
	for _, b := range h.sortedCache {
		cumulative += h.counts[b]
		if cumulative >= target {
			return time.Duration(b) * time.Microsecond
		}
	}
	return time.Duration(h.max)
}

func (h *latencyHistogram) average() time.Duration {
	if h.count == 0 {
		return 0
	}
	return time.Duration(h.sum / h.count)
}

func (h *latencyHistogram) minimum() time.Duration {
	if h.count == 0 || h.min == math.MaxInt64 {
		return 0
	}
	return time.Duration(h.min)
}

func (h *latencyHistogram) maximum() time.Duration {
	if h.count == 0 {
		return 0
	}
	return time.Duration(h.max)
}

// mergeFrom folds another histogram's buckets and aggregates into h. Used to
// combine per-shard histograms into a single view for Snapshot.
func (h *latencyHistogram) mergeFrom(o *latencyHistogram) {
	if o.count == 0 {
		return
	}
	for b, c := range o.counts {
		h.counts[b] += c
	}
	h.count += o.count
	h.sum += o.sum
	if o.min < h.min {
		h.min = o.min
	}
	if o.max > h.max {
		h.max = o.max
	}
	h.sortedCacheDirty = true
}

// TimingBreakdown captures the phases of an HTTP request.
type TimingBreakdown struct {
	DNSLookup  time.Duration `json:"dns_ms"`
	TCPConnect time.Duration `json:"tcp_ms"`
	TLSShake   time.Duration `json:"tls_ms"`
	TTFB       time.Duration `json:"ttfb_ms"`    // time to first byte
	Transfer   time.Duration `json:"transfer_ms"`
}

// RateLimitInfo captures rate limiting behavior detected from HTTP 429 responses.
type RateLimitInfo struct {
	Total429s          int              `json:"total_429s"`
	FirstHitAt         time.Duration    `json:"first_hit_at"`
	RetryAfterSec      float64          `json:"retry_after_sec"`
	RateLimitLimit     string           `json:"rate_limit_limit"`
	RateLimitRemaining string           `json:"rate_limit_remaining"`
	RateLimitReset     string           `json:"rate_limit_reset"`
	HitsOverTime       []RateLimitPoint `json:"hits_over_time"`
}

// RateLimitPoint captures 429s and total requests per second.
type RateLimitPoint struct {
	TimeSec  float64 `json:"t"`
	Count429 int     `json:"c"`
	TotalReq int     `json:"r"`
}

// TLSInfo captures TLS handshake timing and certificate details.
type TLSInfo struct {
	HandshakeTime time.Duration
	Protocol      string    // "TLS 1.3", "TLS 1.2", etc.
	CipherSuite   string
	ServerName    string
	NotBefore     time.Time
	NotAfter      time.Time
	Issuer        string
}

// TimelinePoint captures a periodic snapshot of metrics for charting.
type TimelinePoint struct {
	Timestamp   time.Duration `json:"t"`      // offset from start
	RPS         float64       `json:"rps"`    // interval RPS
	AvgLatency  float64       `json:"lat_ms"` // in milliseconds
	Concurrency int           `json:"conc"`   // active workers
	TotalReqs   int           `json:"reqs"`
	Errors      int           `json:"errs"`
	CircuitOpen bool          `json:"circuit_open,omitempty"` // true if circuit breaker is open
}

// CircuitEvent records when the circuit breaker changes state.
type CircuitEvent struct {
	Timestamp time.Duration `json:"t"`
	State     string        `json:"state"` // "open", "half-open", "closed"
	Reason    string        `json:"reason"`
}


// metricShard holds the parts of recording that need a lock (the latency
// histogram and the per-code / per-second maps). Sharding these across many
// independent locks removes the single-mutex bottleneck that otherwise
// serializes every worker on every request. Padded to a cache line to avoid
// false sharing between adjacent shards.
type metricShard struct {
	mu          sync.Mutex
	hist        *latencyHistogram
	statusCodes map[int]int
	reqsPerSec  map[int]int // second -> total requests (for 429 correlation)
	_           [24]byte    // pad toward a cache line
}

type Metrics struct {
	startTime time.Time

	// Hot counters — atomic, no lock on the request path.
	totalReqs atomic.Int64
	errors    atomic.Int64
	corrected atomic.Int64 // coordinated-omission-corrected synthetic records
	valFails  atomic.Int64

	// Request timing breakdown — atomic nanosecond sums.
	timingCount atomic.Int64
	dnsSum      atomic.Int64
	tcpSum      atomic.Int64
	tlsSum      atomic.Int64
	ttfbSum     atomic.Int64
	transferSum atomic.Int64

	// TLS handshake aggregate + first-seen certificate info.
	tlsHandshakeSum   atomic.Int64
	tlsHandshakeCount atomic.Int64
	tlsInfo           atomic.Pointer[TLSInfo]

	// Sharded histogram + status/second maps.
	shards   []*metricShard
	shardMask uint32

	// Active concurrency (for the timeline) and warm-up cutoff (unix nanos, 0=off).
	activeConcurrency atomic.Int64
	warmupEndNanos    atomic.Int64

	// Circuit breaker events — rare, guarded by a dedicated lock.
	ceMu          sync.Mutex
	circuitEvents []CircuitEvent

	// Rate-limit headers + 429 correlation — only touched on 429s (rare).
	rlMu          sync.Mutex
	total429s     int
	firstHitAt    time.Duration
	retryAfter    string
	limitHeader   string
	remainHeader  string
	resetHeader   string
	hits429PerSec map[int]int

	// Timeline — single writer (RecordTimelinePoint, every 500ms) + Snapshot reader.
	tlMu                 sync.Mutex
	timeline             []TimelinePoint
	lastTimelineReqs     int64
	lastTimelineTime     time.Time
	lastTimelineLatSum   int64
	lastTimelineLatCount int64
}

// maxTimelinePoints bounds timeline memory (and the Snapshot copy cost) on very
// long runs. When exceeded the series is downsampled 2:1, halving resolution
// while still covering the full run.
const maxTimelinePoints = 7200

// shardFor returns the shard for a given worker/shard hint.
func (m *Metrics) shardFor(hint int) *metricShard {
	return m.shards[uint32(hint)&m.shardMask]
}

// NumShards reports how many histogram shards exist so callers can spread their
// worker load across them.
func (m *Metrics) NumShards() int { return len(m.shards) }

type Snapshot struct {
	Duration    time.Duration
	TotalReqs   int
	Errors      int
	RPS         float64
	AvgLatency  time.Duration
	P50Latency  time.Duration
	P95Latency  time.Duration
	P99Latency  time.Duration
	MinLatency  time.Duration
	MaxLatency  time.Duration
	StatusCodes map[int]int
	Latencies       []time.Duration
	Timeline        []TimelinePoint
	TLSInfo         *TLSInfo      `json:"-"`
	AvgTLSHandshake time.Duration
	RateLimit       *RateLimitInfo `json:"-"`
	CircuitEvents   []CircuitEvent
	CorrectedReqs      int              // number of CO-corrected latency records added
	ValidationFailures int              // number of requests that failed validation
	AvgTiming          *TimingBreakdown // average timing breakdown (nil if no data)
}

func New() *Metrics {
	now := time.Now()

	// Size the shard array to comfortably exceed the CPU count so concurrently
	// running workers rarely collide on the same shard lock. Power of two so we
	// can mask instead of modulo.
	n := 1
	for n < 4*runtime.GOMAXPROCS(0) {
		n <<= 1
	}
	if n < 8 {
		n = 8
	}
	if n > 256 {
		n = 256
	}
	shards := make([]*metricShard, n)
	for i := range shards {
		shards[i] = &metricShard{
			hist:        newLatencyHistogram(),
			statusCodes: make(map[int]int),
			reqsPerSec:  make(map[int]int),
		}
	}

	return &Metrics{
		startTime:        now,
		shards:           shards,
		shardMask:        uint32(n - 1),
		timeline:         make([]TimelinePoint, 0, 256),
		lastTimelineTime: now,
	}
}

func (m *Metrics) warmingUp() bool {
	end := m.warmupEndNanos.Load()
	return end != 0 && time.Now().UnixNano() < end
}

// Record ingests one request result. shardHint spreads the per-request lock
// load across shards; callers on the hot path should pass a stable per-worker
// value (see RecordAt). Record with an internal round-robin hint is kept for
// low-frequency callers and tests.
func (m *Metrics) Record(statusCode int, latency time.Duration, isError bool) {
	m.RecordAt(int(m.totalReqs.Load()), statusCode, latency, isError)
}

// RecordAt is the hot-path recorder. shard is a stable per-worker index.
func (m *Metrics) RecordAt(shard, statusCode int, latency time.Duration, isError bool) {
	if m.warmingUp() {
		return
	}

	m.totalReqs.Add(1)
	if isError {
		m.errors.Add(1)
	}

	sec := int(time.Since(m.startTime).Seconds())
	sh := m.shardFor(shard)
	sh.mu.Lock()
	sh.hist.record(latency)
	sh.statusCodes[statusCode]++
	sh.reqsPerSec[sec]++
	sh.mu.Unlock()
}

// RecordCircuitEvent records a circuit breaker state change.
func (m *Metrics) RecordCircuitEvent(state, reason string) {
	m.ceMu.Lock()
	defer m.ceMu.Unlock()
	m.circuitEvents = append(m.circuitEvents, CircuitEvent{
		Timestamp: time.Since(m.startTime),
		State:     state,
		Reason:    reason,
	})
}

// RecordRateLimit records a 429 response and associated rate limit headers.
func (m *Metrics) RecordRateLimit(retryAfter, limitHeader, remainingHeader, resetHeader string) {
	elapsed := time.Since(m.startTime)
	m.rlMu.Lock()
	defer m.rlMu.Unlock()

	m.total429s++
	if m.firstHitAt == 0 {
		m.firstHitAt = elapsed
	}
	if retryAfter != "" && m.retryAfter == "" {
		m.retryAfter = retryAfter
	}
	if limitHeader != "" {
		m.limitHeader = limitHeader
	}
	if remainingHeader != "" {
		m.remainHeader = remainingHeader
	}
	if resetHeader != "" {
		m.resetHeader = resetHeader
	}
	if m.hits429PerSec == nil {
		m.hits429PerSec = make(map[int]int)
	}
	m.hits429PerSec[int(elapsed.Seconds())]++
}

// RecordTLS records TLS handshake information. The full TLSInfo is captured once
// from the first TLS connection; handshake durations are aggregated for averaging.
func (m *Metrics) RecordTLS(info TLSInfo) {
	if m.tlsInfo.Load() == nil {
		cp := info
		m.tlsInfo.CompareAndSwap(nil, &cp)
	}
	m.tlsHandshakeSum.Add(int64(info.HandshakeTime))
	m.tlsHandshakeCount.Add(1)
}

// RecordCorrected records a synthetic latency entry for coordinated omission
// correction. These affect percentiles but not request count or RPS.
func (m *Metrics) RecordCorrected(latency time.Duration) {
	m.RecordCorrectedAt(int(m.totalReqs.Load()), latency)
}

// RecordCorrectedAt is the hot-path variant with an explicit shard hint.
func (m *Metrics) RecordCorrectedAt(shard int, latency time.Duration) {
	sh := m.shardFor(shard)
	sh.mu.Lock()
	sh.hist.record(latency)
	sh.mu.Unlock()
	m.corrected.Add(1)
}

// RecordValidationFailure increments the validation failure counter.
func (m *Metrics) RecordValidationFailure() {
	m.valFails.Add(1)
}

// SetWarmup configures a warm-up period during which requests are not recorded.
func (m *Metrics) SetWarmup(d time.Duration) {
	if d > 0 {
		m.warmupEndNanos.Store(m.startTime.Add(d).UnixNano())
	}
}

// RecordTiming records request timing breakdown (DNS, TCP, TLS, TTFB, transfer).
func (m *Metrics) RecordTiming(t TimingBreakdown) {
	m.timingCount.Add(1)
	m.dnsSum.Add(int64(t.DNSLookup))
	m.tcpSum.Add(int64(t.TCPConnect))
	m.tlsSum.Add(int64(t.TLSShake))
	m.ttfbSum.Add(int64(t.TTFB))
	m.transferSum.Add(int64(t.Transfer))
}

// TLSDetails returns the captured TLS info, or nil if no TLS connection was made.
func (m *Metrics) TLSDetails() *TLSInfo {
	return m.tlsInfo.Load()
}

// SetConcurrency sets the active concurrency level for timeline tracking.
func (m *Metrics) SetConcurrency(n int) {
	m.activeConcurrency.Store(int64(n))
}

// histTotals merges the sharded latency histograms into one and also returns
// the aggregate sample count and nanosecond sum. Caller owns the returned
// histogram (do not retain shard references).
func (m *Metrics) histTotals() (merged *latencyHistogram, sum, count int64) {
	merged = newLatencyHistogram()
	for _, sh := range m.shards {
		sh.mu.Lock()
		merged.mergeFrom(sh.hist)
		sh.mu.Unlock()
	}
	return merged, merged.sum, merged.count
}

// Totals returns the current cumulative counters plus the aggregate latency sum
// (nanoseconds) and sample count. Diffing two calls over a window yields that
// window's RPS, error rate, and average latency — used by the capacity probe.
func (m *Metrics) Totals() (reqs, errs, latSumNanos, latCount int64) {
	_, sum, count := m.histTotals()
	return m.totalReqs.Load(), m.errors.Load(), sum, count
}

// RecordTimelinePoint captures a periodic snapshot. Call externally (e.g. every 500ms).
func (m *Metrics) RecordTimelinePoint() {
	now := time.Now()
	elapsed := now.Sub(m.startTime)

	total := m.totalReqs.Load()
	errs := m.errors.Load()
	_, latSum, latCount := m.histTotals()

	m.tlMu.Lock()
	defer m.tlMu.Unlock()

	intervalDur := now.Sub(m.lastTimelineTime).Seconds()
	intervalReqs := total - m.lastTimelineReqs

	var intervalRPS float64
	if intervalDur > 0 {
		intervalRPS = float64(intervalReqs) / intervalDur
	}

	// Interval average latency: mean of just the requests recorded since the
	// last timeline point, so the live chart reflects current latency rather
	// than a cumulative average that flattens out over the run.
	intervalLatCount := latCount - m.lastTimelineLatCount
	intervalLatSum := latSum - m.lastTimelineLatSum
	var avgLatMs float64
	if intervalLatCount > 0 {
		avgLatMs = float64(intervalLatSum) / float64(intervalLatCount) / 1e6
	} else if latCount > 0 {
		avgLatMs = float64(latSum) / float64(latCount) / 1e6
	}

	m.timeline = append(m.timeline, TimelinePoint{
		Timestamp:   elapsed,
		RPS:         intervalRPS,
		AvgLatency:  avgLatMs,
		Concurrency: int(m.activeConcurrency.Load()),
		TotalReqs:   int(total),
		Errors:      int(errs),
	})
	// Bound memory on very long runs by downsampling 2:1 when we hit the cap.
	if len(m.timeline) > maxTimelinePoints {
		half := m.timeline[:0]
		for i := 0; i < len(m.timeline); i += 2 {
			half = append(half, m.timeline[i])
		}
		m.timeline = half
	}

	m.lastTimelineReqs = total
	m.lastTimelineTime = now
	m.lastTimelineLatSum = latSum
	m.lastTimelineLatCount = latCount
}

// Timeline returns a copy of the collected timeline points.
func (m *Metrics) Timeline() []TimelinePoint {
	m.tlMu.Lock()
	defer m.tlMu.Unlock()
	cp := make([]TimelinePoint, len(m.timeline))
	copy(cp, m.timeline)
	return cp
}

func (m *Metrics) Snapshot() Snapshot {
	elapsed := time.Since(m.startTime)
	snap := Snapshot{
		Duration:    elapsed,
		TotalReqs:   int(m.totalReqs.Load()),
		Errors:      int(m.errors.Load()),
		StatusCodes: make(map[int]int),
	}

	// Merge sharded histogram + status codes + per-second request counts.
	hist := newLatencyHistogram()
	reqsPerSec := make(map[int]int)
	for _, sh := range m.shards {
		sh.mu.Lock()
		hist.mergeFrom(sh.hist)
		for k, v := range sh.statusCodes {
			snap.StatusCodes[k] += v
		}
		for s, c := range sh.reqsPerSec {
			reqsPerSec[s] += c
		}
		sh.mu.Unlock()
	}

	// Copy timeline.
	m.tlMu.Lock()
	snap.Timeline = make([]TimelinePoint, len(m.timeline))
	copy(snap.Timeline, m.timeline)
	m.tlMu.Unlock()

	// Copy circuit events.
	m.ceMu.Lock()
	snap.CircuitEvents = make([]CircuitEvent, len(m.circuitEvents))
	copy(snap.CircuitEvents, m.circuitEvents)
	m.ceMu.Unlock()

	snap.TLSInfo = m.tlsInfo.Load()
	if hc := m.tlsHandshakeCount.Load(); hc > 0 {
		snap.AvgTLSHandshake = time.Duration(m.tlsHandshakeSum.Load() / hc)
	}

	snap.CorrectedReqs = int(m.corrected.Load())
	snap.ValidationFailures = int(m.valFails.Load())

	if tc := m.timingCount.Load(); tc > 0 {
		snap.AvgTiming = &TimingBreakdown{
			DNSLookup:  time.Duration(m.dnsSum.Load() / tc),
			TCPConnect: time.Duration(m.tcpSum.Load() / tc),
			TLSShake:   time.Duration(m.tlsSum.Load() / tc),
			TTFB:       time.Duration(m.ttfbSum.Load() / tc),
			Transfer:   time.Duration(m.transferSum.Load() / tc),
		}
	}

	if hist.count == 0 {
		return snap
	}

	if elapsed.Seconds() > 0 {
		snap.RPS = float64(snap.TotalReqs) / elapsed.Seconds()
	}
	snap.AvgLatency = hist.average()
	snap.MinLatency = hist.minimum()
	snap.MaxLatency = hist.maximum()
	snap.P50Latency = hist.percentile(50)
	snap.P95Latency = hist.percentile(95)
	snap.P99Latency = hist.percentile(99)

	// Populate rate limit info if any 429s were recorded.
	m.rlMu.Lock()
	if m.total429s > 0 {
		rl := &RateLimitInfo{
			Total429s:          m.total429s,
			FirstHitAt:         m.firstHitAt,
			RateLimitLimit:     m.limitHeader,
			RateLimitRemaining: m.remainHeader,
			RateLimitReset:     m.resetHeader,
		}
		if m.retryAfter != "" {
			if v, err := strconv.ParseFloat(m.retryAfter, 64); err == nil {
				rl.RetryAfterSec = v
			}
		}
		maxSec := 0
		for s := range reqsPerSec {
			if s > maxSec {
				maxSec = s
			}
		}
		for s := range m.hits429PerSec {
			if s > maxSec {
				maxSec = s
			}
		}
		for s := 0; s <= maxSec; s++ {
			c429 := m.hits429PerSec[s]
			rTotal := reqsPerSec[s]
			if c429 > 0 || rTotal > 0 {
				rl.HitsOverTime = append(rl.HitsOverTime, RateLimitPoint{
					TimeSec:  float64(s),
					Count429: c429,
					TotalReq: rTotal,
				})
			}
		}
		snap.RateLimit = rl
	}
	m.rlMu.Unlock()

	return snap
}
