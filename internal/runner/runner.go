package runner

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mertgundoganx/gload/internal/faker"
	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/metrics"
	"github.com/mertgundoganx/gload/internal/plugin"
	"github.com/mertgundoganx/gload/pkg/config"
)

// dnsCacheEntry holds cached DNS lookup results with an expiration time.
type dnsCacheEntry struct {
	addrs   []string
	expires time.Time
}

// dnsCacheResolver is a simple DNS cache that avoids repeated DNS lookups
// during load tests. Entries expire after 60 seconds.
type dnsCacheResolver struct {
	mu    sync.RWMutex
	cache map[string]dnsCacheEntry
}

func (r *dnsCacheResolver) lookup(host string) ([]string, error) {
	r.mu.RLock()
	entry, ok := r.cache[host]
	r.mu.RUnlock()

	if ok && time.Now().Before(entry.expires) {
		return entry.addrs, nil
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[host] = dnsCacheEntry{addrs: addrs, expires: time.Now().Add(60 * time.Second)}
	r.mu.Unlock()

	return addrs, nil
}

func (r *dnsCacheResolver) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	addrs, err := r.lookup(host)
	if err != nil {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
	}

	// Try cached addresses.
	for _, a := range addrs {
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(a, port))
		if err == nil {
			return conn, nil
		}
	}

	// Fallback to original address.
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
}

// Circuit breaker states.
const (
	cbClosed   int32 = 0
	cbOpen     int32 = 1
	cbHalfOpen int32 = 2
)

// circuitBreaker implements a simple circuit breaker. The hot path (every
// request calls shouldAllow + recordSuccess/recordError) is lock-free via
// atomics; the mutex is only taken on the rare state transitions so thousands
// of workers don't serialize on it per request.
type circuitBreaker struct {
	state           atomic.Int32
	consecutiveErrs atomic.Int64
	openUntilNanos  atomic.Int64
	mu              sync.Mutex // guards transitions only
	threshold       int64
	cooldown        time.Duration
}

func newCircuitBreaker() *circuitBreaker {
	cb := &circuitBreaker{
		threshold: 10,
		cooldown:  3 * time.Second,
	}
	cb.state.Store(cbClosed)
	return cb
}

func (cb *circuitBreaker) recordSuccess() {
	switch cb.state.Load() {
	case cbClosed:
		// Common case: reset the error streak without locking.
		cb.consecutiveErrs.Store(0)
	case cbHalfOpen:
		cb.mu.Lock()
		if cb.state.Load() == cbHalfOpen {
			cb.state.Store(cbClosed)
			cb.consecutiveErrs.Store(0)
		}
		cb.mu.Unlock()
	}
}

func (cb *circuitBreaker) recordError() bool {
	n := cb.consecutiveErrs.Add(1)
	st := cb.state.Load()
	if st == cbClosed && n >= cb.threshold {
		cb.mu.Lock()
		tripped := false
		if cb.state.Load() == cbClosed {
			cb.state.Store(cbOpen)
			cb.openUntilNanos.Store(time.Now().Add(cb.cooldown).UnixNano())
			tripped = true
		}
		cb.mu.Unlock()
		return tripped
	}
	if st == cbHalfOpen {
		cb.mu.Lock()
		if cb.state.Load() == cbHalfOpen {
			cb.state.Store(cbOpen)
			cb.openUntilNanos.Store(time.Now().Add(cb.cooldown).UnixNano())
		}
		cb.mu.Unlock()
	}
	return false
}

// shouldAllow returns true if a request should proceed. Lock-free in the common
// closed state.
func (cb *circuitBreaker) shouldAllow() bool {
	switch cb.state.Load() {
	case cbClosed, cbHalfOpen:
		return true
	case cbOpen:
		if time.Now().UnixNano() > cb.openUntilNanos.Load() {
			// Transition to half-open to allow a single probe.
			cb.state.CompareAndSwap(cbOpen, cbHalfOpen)
			return true
		}
		return false
	}
	return true
}

func (cb *circuitBreaker) getState() string {
	switch cb.state.Load() {
	case cbOpen:
		return "open"
	case cbHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

type Runner struct {
	cfg          *config.Config
	client       *http.Client
	Metrics      *metrics.Metrics
	Done         chan struct{}
	dataCounter  atomic.Int64 // round-robin counter for DataSource
	cb           *circuitBreaker
	protocol     plugin.Protocol // non-nil when using a non-HTTP protocol plugin
}

// maxValidationBody bounds how much of a response body we buffer for
// validation/extraction so a large target response cannot exhaust memory.
const maxValidationBody = 10 << 20 // 10 MiB

// timingSampleRate controls how often per-request timing (DNS/TCP/TLS/TTFB)
// tracing is attached. Tracing allocates per request, so we sample ~1/N of
// requests; the reported breakdown is an average over the sampled requests.
const timingSampleRate = 16

// UserAgent is sent on every request that doesn't already carry one, so target
// operators can identify the traffic as gload. main stamps it with the release
// version at startup; a user-supplied User-Agent header always takes precedence.
var UserAgent = "gload/1.0"

// workerCtx carries per-worker state down the request path: a stable metrics
// shard (to spread histogram lock load) and a private RNG (so weighted-step
// selection doesn't hit the globally-locked math/rand source).
type workerCtx struct {
	shard     int
	rng       *rand.Rand
	reqSeq    int // for timing sampling
}

func (r *Runner) newWorkerCtx(shard int) *workerCtx {
	return &workerCtx{
		shard: shard,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano() ^ (int64(shard)+1)*2654435761)),
	}
}

// traceThisRequest reports whether the next request should carry httptrace.
func (wc *workerCtx) traceThisRequest() bool {
	wc.reqSeq++
	return wc.reqSeq%timingSampleRate == 1 // always samples the first request
}

func New(cfg *config.Config) *Runner {
	transport := &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   cfg.DisableKeepAlive,
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: false},
	}

	// Set defaults.
	if transport.MaxIdleConns == 0 {
		transport.MaxIdleConns = 100
	}
	if transport.MaxIdleConnsPerHost == 0 {
		transport.MaxIdleConnsPerHost = cfg.Concurrency
	}

	// HTTP/2 support: Go enables HTTP/2 automatically for HTTPS when
	// MaxConnsPerHost is not set. Setting MaxConnsPerHost disables HTTP/2
	// multiplexing. So we only set MaxConnsPerHost when the user explicitly
	// disables HTTP/2 and keep-alive is also disabled.
	if !cfg.HTTP2 && cfg.DisableKeepAlive {
		transport.MaxConnsPerHost = cfg.Concurrency
	}

	// Optimize transport buffers for maximum throughput.
	transport.WriteBufferSize = 32 * 1024
	transport.ReadBufferSize = 32 * 1024
	transport.ForceAttemptHTTP2 = !cfg.DisableKeepAlive && cfg.HTTP2
	transport.ResponseHeaderTimeout = cfg.Timeout

	// Fail fast on connect/handshake. A hand-built http.Transport has these
	// unset (0 = unbounded), so if the target or the network becomes
	// unreachable mid-test, connections would otherwise hang for the full
	// request timeout — piling up goroutines and sockets. Bounding them keeps
	// the load generator (and the machine's network stack) from drowning.
	const connectTimeout = 10 * time.Second
	transport.TLSHandshakeTimeout = connectTimeout

	// DNS caching: use a custom dialer that caches DNS lookups. Otherwise use a
	// plain dialer — either way with a bounded connect timeout.
	if cfg.DNSCacheEnabled {
		resolver := &dnsCacheResolver{
			cache: make(map[string]dnsCacheEntry),
		}
		transport.DialContext = resolver.dialContext
	} else {
		transport.DialContext = (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}

	r := &Runner{
		cfg:     cfg,
		client:  client,
		Metrics: metrics.New(),
		Done:    make(chan struct{}),
		cb:      newCircuitBreaker(),
	}
	r.initProtocol()
	return r
}

// initProtocol sets up a protocol plugin if the config specifies a non-HTTP protocol.
func (r *Runner) initProtocol() {
	if r.cfg.Protocol == "" || r.cfg.Protocol == "http" {
		return
	}

	// Instantiate the protocol from the shared plugin registry so the runner and
	// the /api/plugins listing stay in sync (single source of truth).
	proto, ok := plugin.Default.NewProtocol(r.cfg.Protocol)
	if !ok {
		return // unknown protocol → fall back to HTTP
	}

	cfg := r.cfg.ProtocolConfig
	if cfg == nil {
		cfg = make(map[string]string)
	}
	// Auto-fill URL from main config if not in protocol config.
	if _, ok := cfg["url"]; !ok {
		cfg["url"] = r.cfg.URL
	}
	if _, ok := cfg["address"]; !ok {
		cfg["address"] = r.cfg.URL
	}

	if err := proto.Init(cfg); err != nil {
		return // fallback to HTTP
	}
	r.protocol = proto
}

// Close cleans up resources held by the runner (e.g. protocol plugin connections).
func (r *Runner) Close() {
	if r.protocol != nil {
		r.protocol.Close()
	}
	// Release pooled keep-alive connections so long-lived servers that run
	// many tests don't leak sockets/file descriptors between runs.
	if r.client != nil {
		r.client.CloseIdleConnections()
	}
}

// warmupConnections pre-establishes HTTP connections to avoid cold-start latency.
func (r *Runner) warmupConnections(ctx context.Context) {
	if r.cfg.WarmupConns <= 0 {
		return
	}
	// Only warm up for HTTP (not protocol plugins).
	if r.protocol != nil {
		return
	}

	n := r.cfg.WarmupConns
	if n > r.cfg.Concurrency {
		n = r.cfg.Concurrency
	}

	logger.Info("warming up connections", logger.Fields("count", n))

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, "HEAD", r.cfg.URL, nil)
			if err != nil {
				return
			}
			for k, v := range r.cfg.Headers {
				req.Header.Set(k, v)
			}
			resp, err := r.client.Do(req)
			if err != nil {
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		logger.Info("connection warm-up complete")
	case <-time.After(10 * time.Second):
		logger.Warn("connection warm-up timed out")
	case <-ctx.Done():
	}
}

// adaptiveController monitors P95 latency and adjusts the active worker count.
type adaptiveController struct {
	mu             sync.Mutex
	enabled        bool
	targetMs       float64
	minWorkers     int
	maxWorkers     int
	currentWorkers int

	scaleDownCount int
	scaleUpCount   int
}

func newAdaptiveController(enabled bool, targetMs float64, maxWorkers int) *adaptiveController {
	if targetMs <= 0 {
		targetMs = 500
	}
	minWorkers := 1
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers
	}

	return &adaptiveController{
		enabled:        enabled,
		targetMs:       targetMs,
		minWorkers:     minWorkers,
		maxWorkers:     maxWorkers,
		currentWorkers: maxWorkers,
	}
}

func (ac *adaptiveController) getWorkerCount() int {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.currentWorkers
}

// evaluate checks current P95 latency and adjusts worker count.
func (ac *adaptiveController) evaluate(p95Ms float64) {
	if !ac.enabled {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if p95Ms > ac.targetMs {
		ac.scaleUpCount = 0
		ac.scaleDownCount++
		if ac.scaleDownCount >= 3 {
			newCount := int(float64(ac.currentWorkers) * 0.8)
			if newCount < ac.minWorkers {
				newCount = ac.minWorkers
			}
			if newCount != ac.currentWorkers {
				ac.currentWorkers = newCount
				ac.scaleDownCount = 0
			}
		}
	} else if p95Ms < ac.targetMs*0.7 {
		ac.scaleDownCount = 0
		ac.scaleUpCount++
		if ac.scaleUpCount >= 5 {
			newCount := int(float64(ac.currentWorkers) * 1.1)
			if newCount > ac.maxWorkers {
				newCount = ac.maxWorkers
			}
			if newCount == ac.currentWorkers {
				newCount++
			}
			if newCount > ac.maxWorkers {
				newCount = ac.maxWorkers
			}
			if newCount != ac.currentWorkers {
				ac.currentWorkers = newCount
				ac.scaleUpCount = 0
			}
		}
	} else {
		ac.scaleDownCount = 0
		ac.scaleUpCount = 0
	}
}

// startRateLimiter creates a goroutine that sends tokens at the given RPS rate.
// Returns a channel workers read from before each request, or nil if rps <= 0.
// The returned stop function must be called to clean up the goroutine.
func startRateLimiter(ctx context.Context, rps int) (tokens <-chan struct{}, stop func()) {
	if rps <= 0 {
		return nil, func() {}
	}

	ch := make(chan struct{}, rps)
	interval := time.Second / time.Duration(rps)
	ticker := time.NewTicker(interval)

	done := make(chan struct{})
	stop = func() {
		ticker.Stop()
		close(done)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				select {
				case ch <- struct{}{}:
				default:
					// Drop token if buffer is full (workers not keeping up).
				}
			}
		}
	}()

	return ch, stop
}

// startTimelineRecorder samples a timeline point every 500ms until stopped.
// Shared by the staged and non-staged paths so both produce live/over-time charts.
func (r *Runner) startTimelineRecorder(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.Metrics.RecordTimelinePoint()
			}
		}
	}()
}

func (r *Runner) Run(ctx context.Context) {
	if len(r.cfg.Stages) > 0 {
		// Record the timeline for the whole staged run so the ramp is visible
		// live and in the report.
		tlCtx, tlCancel := context.WithCancel(ctx)
		r.startTimelineRecorder(tlCtx)
		if r.cfg.OpenModel {
			r.runArrivalStages(ctx) // ramp arrival rate (open model) — e.g. a launch spike
		} else {
			r.runStages(ctx) // ramp concurrency (closed model)
		}
		tlCancel()
		close(r.Done)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, r.cfg.Duration)
	defer cancel()

	// Warm-up
	if r.cfg.WarmupDuration > 0 {
		r.Metrics.SetWarmup(r.cfg.WarmupDuration)
	}

	r.Metrics.SetConcurrency(r.cfg.Concurrency)

	// Timeline recording (stops when the timeout context ends).
	r.startTimelineRecorder(ctx)

	r.warmupConnections(ctx)

	if r.cfg.ArrivalRate > 0 {
		r.runArrivalRate(ctx)
	} else {
		r.runClosedModel(ctx)
	}

	close(r.Done)
}

// runClosedModel is the existing behavior — fixed number of workers sending as fast as possible.
// When adaptive concurrency is enabled, workers dynamically pause/resume based on P95 latency.
func (r *Runner) runClosedModel(ctx context.Context) {
	tokens, stopLimiter := startRateLimiter(ctx, r.cfg.RPS)
	defer stopLimiter()

	ac := newAdaptiveController(r.cfg.AdaptiveConcurrency, r.cfg.AdaptiveTargetMs, r.cfg.Concurrency)

	if ac.enabled {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					snap := r.Metrics.Snapshot()
					p95Ms := float64(snap.P95Latency.Microseconds()) / 1000
					ac.evaluate(p95Ms)
					r.Metrics.SetConcurrency(ac.getWorkerCount())
				}
			}
		}()
	}

	var wg sync.WaitGroup
	for i := 0; i < r.cfg.Concurrency; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			client := r.client
			if r.cfg.CookieJar {
				jar, _ := cookiejar.New(nil)
				client = &http.Client{
					Timeout:   r.cfg.Timeout,
					Transport: r.client.Transport,
					Jar:       jar,
				}
			}
			r.adaptiveWorker(ctx, tokens, client, workerID, ac)
		}()
	}
	wg.Wait()
}

func (r *Runner) adaptiveWorker(ctx context.Context, tokens <-chan struct{}, client *http.Client, id int, ac *adaptiveController) {
	var lastRequestEnd time.Time
	var avgInterval time.Duration
	var requestCount int64
	wc := r.newWorkerCtx(id)

	iterations := r.cfg.RequestsPerIteration
	if iterations <= 0 {
		iterations = 1
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Adaptive check: if this worker ID exceeds current count, sleep
			if ac.enabled && id >= ac.getWorkerCount() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
					continue
				}
			}

			if tokens != nil {
				select {
				case <-ctx.Done():
					return
				case <-tokens:
				}
			}

			now := time.Now()

			// CO correction
			if requestCount > 10 && avgInterval > 0 && !lastRequestEnd.IsZero() {
				gap := now.Sub(lastRequestEnd)
				if gap > avgInterval*3 {
					missedCount := int((gap - avgInterval) / avgInterval)
					if missedCount > 0 && missedCount < 1000 {
						for i := 0; i < missedCount; i++ {
							r.Metrics.RecordCorrectedAt(wc.shard, gap)
						}
					}
				}
			}

			for j := 0; j < iterations; j++ {
				if ctx.Err() != nil {
					return
				}
				r.sendRequestWithClient(ctx, client, wc)
			}

			// Think time
			r.applyThinkTime(ctx)

			requestEnd := time.Now()
			requestCount++
			if !lastRequestEnd.IsZero() {
				interval := requestEnd.Sub(lastRequestEnd)
				if avgInterval == 0 {
					avgInterval = interval
				} else {
					avgInterval = (avgInterval*9 + interval) / 10
				}
			}
			lastRequestEnd = requestEnd
		}
	}
}

// runArrivalRate spawns a new goroutine every 1/ArrivalRate seconds (open model).
// In-flight requests are capped by a semaphore so that a slow target can't cause
// unbounded goroutine/memory growth — the classic open-model failure mode. When
// the cap is hit, the arrival is dropped and counted as an error (overload
// signal) rather than piling up.
func (r *Runner) runArrivalRate(ctx context.Context) {
	interval := time.Second / time.Duration(r.cfg.ArrivalRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Cap in-flight at a generous multiple of the arrival rate (min 1000). This
	// tolerates transient latency spikes but bounds a genuine overload.
	maxInflight := r.cfg.ArrivalRate * 10
	if maxInflight < 1000 {
		maxInflight = 1000
	}
	sem := make(chan struct{}, maxInflight)

	var wg sync.WaitGroup
	var seq atomic.Int64

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			select {
			case sem <- struct{}{}:
				// Slot acquired.
			default:
				// Overloaded: target can't keep up with the arrival rate.
				r.Metrics.Record(0, 0, true)
				continue
			}
			wg.Add(1)
			shard := int(seq.Add(1))
			go func(shard int) {
				defer wg.Done()
				defer func() { <-sem }()
				client := r.client
				if r.cfg.CookieJar {
					jar, _ := cookiejar.New(nil)
					client = &http.Client{Timeout: r.cfg.Timeout, Transport: r.client.Transport, Jar: jar}
				}
				wc := r.newWorkerCtx(shard)
				// Single request (or scenario)
				r.sendRequestWithClient(ctx, client, wc)
				// Optional think time before VU "exits"
				r.applyThinkTime(ctx)
			}(shard)
		}
	}
}

// runArrivalStages runs the open model with a ramping arrival rate: each stage's
// Target is a target arrival rate (requests/sec), linearly interpolated from the
// previous stage's target over the stage duration. This models a real launch
// spike — a "thundering herd" of independent arrivals that keep coming even as
// the target slows down. In-flight requests are capped so a slow target can't
// blow up the load generator; over-cap arrivals are dropped and counted as
// errors (an overload signal).
func (r *Runner) runArrivalStages(ctx context.Context) {
	var total time.Duration
	peakRate := 0
	for _, stage := range r.cfg.Stages {
		total += stage.Duration
		if stage.Target > peakRate {
			peakRate = stage.Target
		}
	}
	ctx, cancel := context.WithTimeout(ctx, total)
	defer cancel()

	var rateMilli atomic.Int64 // current arrival rate × 1000 (req/s)
	var inFlight atomic.Int64

	maxInflight := peakRate * 10
	if maxInflight < 1000 {
		maxInflight = 1000
	}
	sem := make(chan struct{}, maxInflight)
	var wg sync.WaitGroup

	// Controller: interpolate the arrival rate across stages.
	ctrlDone := make(chan struct{})
	go func() {
		defer close(ctrlDone)
		from := 0.0
		ticker := time.NewTicker(stageRampInterval)
		defer ticker.Stop()
		for _, stage := range r.cfg.Stages {
			if ctx.Err() != nil {
				return
			}
			to := float64(stage.Target)
			start := time.Now()
			for {
				elapsed := time.Since(start)
				if elapsed >= stage.Duration {
					break
				}
				frac := float64(elapsed) / float64(stage.Duration)
				rateMilli.Store(int64((from + (to-from)*frac) * 1000))
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
			rateMilli.Store(int64(to * 1000))
			from = to
		}
	}()

	launch := func() {
		select {
		case sem <- struct{}{}:
		default:
			r.Metrics.Record(0, 0, true) // over capacity → dropped arrival (overload)
			return
		}
		wg.Add(1)
		shard := int(inFlight.Add(1))
		go func(shard int) {
			defer wg.Done()
			defer func() { <-sem; inFlight.Add(-1) }()
			client := r.client
			if r.cfg.CookieJar {
				jar, _ := cookiejar.New(nil)
				client = &http.Client{Timeout: r.cfg.Timeout, Transport: r.client.Transport, Jar: jar}
			}
			r.sendRequestWithClient(ctx, client, r.newWorkerCtx(shard))
		}(shard)
	}

	// Launcher: fire the fractional number of arrivals due each tick.
	const launchTick = 10 * time.Millisecond
	lt := time.NewTicker(launchTick)
	defer lt.Stop()
	acc := 0.0
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ctrlDone:
			wg.Wait()
			return
		case <-lt.C:
			rate := float64(rateMilli.Load()) / 1000.0
			r.Metrics.SetConcurrency(int(inFlight.Load()))
			acc += rate * launchTick.Seconds()
			n := int(acc)
			acc -= float64(n)
			for i := 0; i < n; i++ {
				launch()
			}
		}
	}
}

// stageRampInterval is how often the active worker count is recomputed while
// interpolating within a stage. Small enough for smooth ramps, large enough to
// be negligible overhead.
const stageRampInterval = 200 * time.Millisecond

// runStages executes staged load with true linear ramping (k6-style): within
// each stage the active worker count is interpolated from the previous stage's
// target to this stage's target over the stage's duration. A stage whose target
// equals the previous one holds steady; a shorter ramp to a higher target is a
// spike. A fixed pool of `peak` workers park/activate against an atomically
// updated desired count, so ramps up *and* down (and repeated spikes) work.
func (r *Runner) runStages(ctx context.Context) {
	var totalDuration time.Duration
	peak := 0
	for _, stage := range r.cfg.Stages {
		totalDuration += stage.Duration
		if stage.Target > peak {
			peak = stage.Target
		}
	}
	if peak < 1 {
		peak = 1
	}

	ctx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	var (
		desired     atomic.Int64 // current interpolated worker count
		tokens      <-chan struct{}
		tokensMu    sync.RWMutex
		stopLimiter = func() {}
	)

	// Controller: linearly interpolate the desired worker count across stages,
	// and swap the rate limiter at each stage boundary.
	ctrlDone := make(chan struct{})
	go func() {
		defer close(ctrlDone)
		from := 0.0 // ramp the first stage up from zero
		ticker := time.NewTicker(stageRampInterval)
		defer ticker.Stop()
		for _, stage := range r.cfg.Stages {
			if ctx.Err() != nil {
				return
			}
			stopLimiter()
			tokensMu.Lock()
			tokens, stopLimiter = startRateLimiter(ctx, stage.RPS)
			tokensMu.Unlock()

			to := float64(stage.Target)
			start := time.Now()
			for {
				elapsed := time.Since(start)
				if elapsed >= stage.Duration {
					break
				}
				frac := float64(elapsed) / float64(stage.Duration)
				cur := int64(from + (to-from)*frac + 0.5)
				desired.Store(cur)
				r.Metrics.SetConcurrency(int(cur))
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
			desired.Store(int64(to))
			r.Metrics.SetConcurrency(int(to))
			from = to
		}
	}()

	// Fixed worker pool. Each worker runs when its id < desired, else parks.
	var wg sync.WaitGroup
	for i := 0; i < peak; i++ {
		wg.Add(1)
		id := int64(i)
		go func() {
			defer wg.Done()
			wc := r.newWorkerCtx(int(id))
			for {
				if ctx.Err() != nil {
					return
				}
				if id >= desired.Load() {
					// Parked: not currently in the active set.
					select {
					case <-ctx.Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
					continue
				}

				tokensMu.RLock()
				tok := tokens
				tokensMu.RUnlock()
				if tok != nil {
					select {
					case <-ctx.Done():
						return
					case <-tok:
					}
				}
				if id >= desired.Load() {
					continue
				}
				r.sendRequestWithClient(ctx, r.client, wc)
				// Think time between iterations, so a staged ramp models real
				// users (who pause between actions) rather than each VU firing
				// flat-out. No-op when think time is unset.
				r.applyThinkTime(ctx)
			}
		}()
	}

	<-ctrlDone
	desired.Store(0)
	stopLimiter()
	cancel()
	wg.Wait()
}

// applyThinkTime pauses for the configured think time between requests.
func (r *Runner) applyThinkTime(ctx context.Context) {
	if r.cfg.ThinkTime <= 0 {
		return
	}
	delay := r.cfg.ThinkTime
	if r.cfg.ThinkTimeMax > r.cfg.ThinkTime {
		spread := r.cfg.ThinkTimeMax - r.cfg.ThinkTime
		delay += time.Duration(rand.Int63n(int64(spread)))
	}
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}

func (r *Runner) sendRequestWithClient(ctx context.Context, client *http.Client, wc *workerCtx) {
	if len(r.cfg.Steps) > 0 {
		// Check if any step has weights — if so, use weighted random selection
		hasWeights := false
		totalWeight := 0
		for _, s := range r.cfg.Steps {
			if s.Weight > 0 {
				hasWeights = true
				totalWeight += s.Weight
			}
		}

		if hasWeights && totalWeight > 0 {
			r.sendWeightedRequest(ctx, client, totalWeight, wc)
		} else {
			r.sendScenarioWithClient(ctx, client, wc)
		}
		return
	}

	url := r.cfg.URL
	bodyStr := r.cfg.Body

	// Apply dynamic data via round-robin if DataSource is configured.
	if len(r.cfg.DataSource) > 0 {
		idx := r.dataCounter.Add(1) - 1
		data := r.cfg.DataSource[idx%int64(len(r.cfg.DataSource))]
		url = r.templateString(url, data)
		bodyStr = r.templateString(bodyStr, data)
	}

	r.executeSingleRequest(ctx, client, r.cfg.Method, url, bodyStr, r.cfg.Headers, r.cfg.Validations, wc)
}

// sendWeightedRequest picks a random step based on weight and executes it.
func (r *Runner) sendWeightedRequest(ctx context.Context, client *http.Client, totalWeight int, wc *workerCtx) {
	pick := wc.rng.Intn(totalWeight)
	cumulative := 0
	var step config.Step
	for _, s := range r.cfg.Steps {
		cumulative += s.Weight
		if pick < cumulative {
			step = s
			break
		}
	}

	method := step.Method
	if method == "" {
		method = "GET"
	}

	url := step.URL
	bodyStr := step.Body

	// Apply dynamic data via round-robin if DataSource is configured.
	if len(r.cfg.DataSource) > 0 {
		idx := r.dataCounter.Add(1) - 1
		data := r.cfg.DataSource[idx%int64(len(r.cfg.DataSource))]
		url = r.templateString(url, data)
		bodyStr = r.templateString(bodyStr, data)
	}

	r.executeSingleRequest(ctx, client, method, url, bodyStr, step.Headers, step.Validations, wc)
}

// executeSingleRequest handles sending one request with full tracing, timing, circuit breaker, and recording.
func (r *Runner) executeSingleRequest(ctx context.Context, client *http.Client, method, url, bodyStr string, headers map[string]string, validations []config.Validation, wc *workerCtx) {
	// Protocol plugin dispatch — bypass HTTP logic entirely.
	if r.protocol != nil {
		start := time.Now()
		result := r.protocol.Execute(ctx)
		latency := result.Latency
		if latency == 0 {
			latency = time.Since(start)
		}
		isError := result.Error != nil || result.StatusCode >= 400
		r.Metrics.RecordAt(wc.shard, result.StatusCode, latency, isError)
		if isError {
			if tripped := r.cb.recordError(); tripped {
				r.Metrics.RecordCircuitEvent("open", "protocol errors")
			}
		} else {
			prevState := r.cb.getState()
			r.cb.recordSuccess()
			if prevState != "closed" && r.cb.getState() == "closed" {
				r.Metrics.RecordCircuitEvent("closed", "recovered")
			}
		}
		return
	}

	// Circuit breaker check - wait if open
	for !r.cb.shouldAllow() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			// keep checking
		}
	}

	// Apply data generation and resolve environment variables — but only when
	// the string actually contains template/env markers, so static requests
	// skip the faker/env scan and its allocations on every request.
	if strings.ContainsAny(url, "{$") {
		url = faker.Generate(r.resolveEnvVars(url))
	}
	if strings.ContainsAny(bodyStr, "{$") {
		bodyStr = faker.Generate(r.resolveEnvVars(bodyStr))
	}

	var body io.Reader
	var multipartContentType string

	if len(r.cfg.FormFields) > 0 {
		body, multipartContentType = r.buildMultipartBody()
	} else if bodyStr != "" {
		body = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		r.Metrics.RecordAt(wc.shard, 0, 0, true)
		return
	}

	if multipartContentType != "" {
		req.Header.Set("Content-Type", multipartContentType)
	}

	for k, v := range headers {
		if strings.ContainsAny(v, "{$") {
			v = faker.Generate(r.resolveEnvVars(v))
		}
		req.Header.Set(k, v)
	}
	// Brand the traffic unless the caller set their own User-Agent.
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", UserAgent)
	}

	// Attach request timing and TLS tracing — sampled to keep per-request
	// overhead low at high RPS (the breakdown is an average over samples).
	traced := wc.traceThisRequest()
	var timing metrics.TimingBreakdown
	var dnsStart, connectStart, tlsStart, gotConnTime time.Time
	// httptrace callbacks are not guaranteed to run on the request goroutine:
	// the transport may fire them from a background dial goroutine that outlives
	// client.Do (e.g. an orphaned racing/pre-dial when a pooled connection wins).
	// Per the httptrace contract, hooks must be safe for concurrent use, so every
	// access to the shared timing state below — including the post-response read —
	// is guarded by this mutex.
	var traceMu sync.Mutex

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			traceMu.Lock()
			dnsStart = time.Now()
			traceMu.Unlock()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			traceMu.Lock()
			timing.DNSLookup = time.Since(dnsStart)
			traceMu.Unlock()
		},
		ConnectStart: func(_, _ string) {
			traceMu.Lock()
			connectStart = time.Now()
			traceMu.Unlock()
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				traceMu.Lock()
				timing.TCPConnect = time.Since(connectStart)
				traceMu.Unlock()
			}
		},
		TLSHandshakeStart: func() {
			traceMu.Lock()
			tlsStart = time.Now()
			traceMu.Unlock()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			if err != nil {
				return
			}
			traceMu.Lock()
			handshakeTime := time.Since(tlsStart)
			timing.TLSShake = handshakeTime
			traceMu.Unlock()

			info := metrics.TLSInfo{
				HandshakeTime: handshakeTime,
			}

			switch state.Version {
			case tls.VersionTLS13:
				info.Protocol = "TLS 1.3"
			case tls.VersionTLS12:
				info.Protocol = "TLS 1.2"
			case tls.VersionTLS11:
				info.Protocol = "TLS 1.1"
			case tls.VersionTLS10:
				info.Protocol = "TLS 1.0"
			default:
				info.Protocol = fmt.Sprintf("TLS 0x%04x", state.Version)
			}

			info.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
			info.ServerName = state.ServerName

			if len(state.PeerCertificates) > 0 {
				cert := state.PeerCertificates[0]
				info.NotBefore = cert.NotBefore
				info.NotAfter = cert.NotAfter
				info.Issuer = cert.Issuer.CommonName
			}

			r.Metrics.RecordTLS(info)
		},
		GotConn: func(_ httptrace.GotConnInfo) {
			traceMu.Lock()
			gotConnTime = time.Now()
			traceMu.Unlock()
		},
		GotFirstResponseByte: func() {
			traceMu.Lock()
			if !gotConnTime.IsZero() {
				timing.TTFB = time.Since(gotConnTime)
			}
			traceMu.Unlock()
		},
	}
	if traced {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		tripped := r.cb.recordError()
		if tripped {
			r.Metrics.RecordCircuitEvent("open", fmt.Sprintf("%d consecutive errors", r.cb.threshold))
		}
		r.Metrics.RecordAt(wc.shard, 0, latency, true)
		return
	}

	// Read body: use ReadAll if validations are configured, otherwise discard.
	// Cap the read so a huge response body from the target can't exhaust memory.
	var respBody []byte
	if len(validations) > 0 {
		respBody, _ = io.ReadAll(io.LimitReader(resp.Body, maxValidationBody))
	} else {
		io.Copy(io.Discard, resp.Body)
	}
	resp.Body.Close()

	// Record timing breakdown (only for sampled requests that carried tracing).
	if traced {
		traceMu.Lock()
		timing.Transfer = time.Since(start) - timing.DNSLookup - timing.TCPConnect - timing.TLSShake - timing.TTFB
		if timing.Transfer < 0 {
			timing.Transfer = 0
		}
		snapshot := timing
		traceMu.Unlock()
		r.Metrics.RecordTiming(snapshot)
	}

	if resp.StatusCode == 429 {
		r.Metrics.RecordRateLimit(
			resp.Header.Get("Retry-After"),
			resp.Header.Get("X-RateLimit-Limit"),
			resp.Header.Get("X-RateLimit-Remaining"),
			resp.Header.Get("X-RateLimit-Reset"),
		)
	}

	isError := resp.StatusCode >= 400

	// Run response validations.
	if len(validations) > 0 && !r.validateResponse(resp, respBody, validations) {
		r.Metrics.RecordValidationFailure()
		isError = true
	}

	if isError && resp.StatusCode >= 500 {
		tripped := r.cb.recordError()
		if tripped {
			r.Metrics.RecordCircuitEvent("open", fmt.Sprintf("server errors (HTTP %d)", resp.StatusCode))
		}
	} else if !isError {
		prevState := r.cb.getState()
		r.cb.recordSuccess()
		if prevState != "closed" && r.cb.getState() == "closed" {
			r.Metrics.RecordCircuitEvent("closed", "service recovered")
		}
	}
	r.Metrics.RecordAt(wc.shard, resp.StatusCode, latency, isError)
}

// validateResponse checks response body/status against the given validations.
// Returns true if all validations pass, false if any fail.
func (r *Runner) validateResponse(resp *http.Response, body []byte, validations []config.Validation) bool {
	for _, v := range validations {
		switch v.Type {
		case "status_code":
			expected, _ := strconv.Atoi(v.Value)
			if resp.StatusCode != expected {
				return false
			}
		case "contains":
			if !strings.Contains(string(body), v.Value) {
				return false
			}
		case "not_contains":
			if strings.Contains(string(body), v.Value) {
				return false
			}
		case "json_path":
			actual := extractJSONPath(body, v.Path)
			if actual != v.Value {
				return false
			}
		case "regex":
			re, err := regexp.Compile(v.Value)
			if err != nil {
				return false
			}
			if !re.Match(body) {
				return false
			}
		}
	}
	return true
}

func (r *Runner) sendScenarioWithClient(ctx context.Context, client *http.Client, wc *workerCtx) {
	var totalLatency time.Duration
	hadError := false
	lastStatusCode := 0

	// Chain variables passed between steps.
	chainVars := make(map[string]string)

	// Pick data item for this scenario iteration (shared across all steps).
	var data map[string]string
	if len(r.cfg.DataSource) > 0 {
		idx := r.dataCounter.Add(1) - 1
		data = r.cfg.DataSource[idx%int64(len(r.cfg.DataSource))]
	}

	for _, step := range r.cfg.Steps {
		if ctx.Err() != nil {
			return
		}

		url := step.URL
		bodyStr := step.Body
		method := step.Method
		if method == "" {
			method = "GET"
		}

		// Apply chain variables from previous steps.
		url = r.templateString(url, chainVars)
		bodyStr = r.templateString(bodyStr, chainVars)

		// Apply dynamic data templating.
		if data != nil {
			url = r.templateString(url, data)
			bodyStr = r.templateString(bodyStr, data)
		}

		// Apply data generation and resolve environment variables.
		url = faker.Generate(r.resolveEnvVars(url))
		bodyStr = faker.Generate(r.resolveEnvVars(bodyStr))

		var body io.Reader
		if bodyStr != "" {
			body = strings.NewReader(bodyStr)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			hadError = true
			break
		}

		// Apply step-level headers with chain vars and env var resolution.
		for k, v := range step.Headers {
			v = r.templateString(v, chainVars)
			req.Header.Set(k, faker.Generate(r.resolveEnvVars(v)))
		}

		start := time.Now()
		resp, err := client.Do(req)
		stepLatency := time.Since(start)
		totalLatency += stepLatency

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			hadError = true
			break
		}

		// Read response body for extraction and validation (bounded).
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxValidationBody))
		resp.Body.Close()

		lastStatusCode = resp.StatusCode

		// Run extractors to populate chain variables.
		for _, ext := range step.Extractors {
			val := extractValue(ext, resp, respBody)
			if val != "" {
				chainVars[ext.Name] = val
			}
		}

		// Run step-level validations.
		if len(step.Validations) > 0 && !r.validateResponse(resp, respBody, step.Validations) {
			r.Metrics.RecordValidationFailure()
			hadError = true
			break
		}

		if resp.StatusCode >= 400 {
			hadError = true
			break
		}
	}

	r.Metrics.RecordAt(wc.shard, lastStatusCode, totalLatency, hadError)
}

// extractValue extracts a value from the HTTP response based on the extractor configuration.
func extractValue(ext config.Extractor, resp *http.Response, body []byte) string {
	switch ext.Source {
	case "header":
		return resp.Header.Get(ext.Path)
	case "cookie":
		for _, c := range resp.Cookies() {
			if c.Name == ext.Path {
				return c.Value
			}
		}
		return ""
	case "body":
		return extractJSONPath(body, ext.Path)
	default:
		return ""
	}
}

// extractJSONPath extracts a value from JSON using simple dot notation.
// Supports: "token", "data.token", "data.items[0].id", "results[2].name"
func extractJSONPath(body []byte, path string) string {
	var obj interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}

	parts := splitJSONPath(path)
	current := obj

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part.key]
		case []interface{}:
			if part.index >= 0 && part.index < len(v) {
				current = v[part.index]
			} else {
				return ""
			}
		default:
			return ""
		}
	}

	// Convert to string.
	switch v := current.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

type pathPart struct {
	key   string
	index int // -1 if not an array access
}

func splitJSONPath(path string) []pathPart {
	var parts []pathPart
	for _, segment := range strings.Split(path, ".") {
		if idx := strings.Index(segment, "["); idx >= 0 {
			key := segment[:idx]
			if key != "" {
				parts = append(parts, pathPart{key: key, index: -1})
			}
			// Parse index.
			idxStr := strings.TrimSuffix(segment[idx+1:], "]")
			n, err := strconv.Atoi(idxStr)
			if err != nil {
				n = 0
			}
			parts = append(parts, pathPart{index: n})
		} else {
			parts = append(parts, pathPart{key: segment, index: -1})
		}
	}
	return parts
}

func (r *Runner) templateString(tpl string, data map[string]string) string {
	result := tpl
	for k, v := range data {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

var envVarRe = regexp.MustCompile(`\{\{env\.([^}]+)\}\}`)

func (r *Runner) resolveEnvVars(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[6 : len(match)-2] // extract VAR_NAME from {{env.VAR_NAME}}
		return os.Getenv(varName)
	})
}

// buildMultipartBody constructs a multipart/form-data request body from the configured form fields.
func (r *Runner) buildMultipartBody() (io.Reader, string) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for _, f := range r.cfg.FormFields {
		value := faker.Generate(r.resolveEnvVars(f.Value))

		if f.IsFile {
			filename := f.Filename
			if filename == "" {
				filename = "upload.bin"
			}
			part, err := writer.CreateFormFile(f.Name, filename)
			if err != nil {
				continue
			}
			part.Write([]byte(value))
		} else {
			writer.WriteField(f.Name, value)
		}
	}

	writer.Close()
	return &buf, writer.FormDataContentType()
}
