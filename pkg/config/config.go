package config

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

// Stage defines a load test phase with a target concurrency and optional RPS limit.
type Stage struct {
	Duration time.Duration `json:"duration"`
	Target   int           `json:"target"`
	RPS      int           `json:"rps"`
}

// Assertion defines a threshold check evaluated after a test completes.
type Assertion struct {
	Metric   string  `json:"metric"`   // "rps", "avg_latency", "p95_latency", "p99_latency", "error_rate", "min_latency", "max_latency"
	Operator string  `json:"operator"` // "gt", "lt", "gte", "lte", "eq"
	Value    float64 `json:"value"`    // threshold value (latencies in ms, error_rate in %, rps as number)
}

// Extractor defines how to extract a value from a response for use in subsequent steps.
type Extractor struct {
	Name   string `json:"name"`   // variable name, e.g. "auth_token"
	Source string `json:"source"` // "body", "header", "cookie"
	// For body: JSONPath-like dot notation, e.g. "data.token" or "results[0].id"
	// For header: header name, e.g. "Authorization"
	// For cookie: cookie name
	Path string `json:"path"`
}

// Validation defines a response body check to run after each request.
type Validation struct {
	Type  string `json:"type"`  // "contains", "not_contains", "json_path", "regex", "status_code"
	Value string `json:"value"` // expected value or pattern
	Path  string `json:"path"`  // for json_path: dot notation path
}

// FormField defines a field in a multipart/form-data request.
type FormField struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	IsFile   bool   `json:"is_file"`   // if true, Value is the file content (or generated)
	Filename string `json:"filename"`  // filename for file fields
	MimeType string `json:"mime_type"` // content type for file fields
}

// Step defines a single HTTP request within a scenario.
type Step struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	Extractors  []Extractor       `json:"extractors"`  // extract values from response for chaining
	Weight      int               `json:"weight"`       // relative weight for random selection (0 = always execute in sequence)
	Validations []Validation      `json:"validations"`  // response validations for this step
}

type Config struct {
	URL         string
	Method      string
	Body        string
	Headers     map[string]string
	Concurrency int
	Duration    time.Duration
	Timeout     time.Duration
	RPS         int     // requests per second limit (0=unlimited)
	Stages      []Stage // if non-empty, overrides Concurrency/Duration/RPS
	OpenModel   bool    // if true, staged Target values are arrival rate (req/s) rather than concurrency
	NoUI        bool
	WebMode     bool
	Port        int
	Steps       []Step                 // if non-empty, run as scenario (sequential steps per iteration)
	DataSource  []map[string]string    `json:"-"` // populated at runtime from service config
	Assertions  []Assertion            // assertions to evaluate after test completes
	CookieJar   bool                   // enable per-worker cookie jar for session persistence
	WorkerMode  bool                   // run as a distributed worker
	Workers     []string               // worker URLs for distributed testing

	// Think time / pacing
	ThinkTime    time.Duration `json:"think_time"`     // fixed delay between requests (0 = none)
	ThinkTimeMax time.Duration `json:"think_time_max"` // if > 0, random delay between ThinkTime and ThinkTimeMax

	// Arrival rate mode (open model)
	ArrivalRate int `json:"arrival_rate"` // new virtual users per second (0 = use concurrency mode)

	// Response body validations for single-URL mode
	Validations []Validation `json:"validations"`

	// Warm-up: exclude first N seconds from results
	WarmupDuration time.Duration `json:"warmup"`

	// Multipart/form-data support
	ContentType string      `json:"content_type"` // "json", "form", "multipart" — default "json"
	FormFields  []FormField `json:"form_fields"`  // for multipart/form-data

	// Protocol plugin support
	Protocol       string            `json:"protocol"`        // "http", "websocket", "graphql", "grpc", "tcp" — default "http"
	ProtocolConfig map[string]string `json:"protocol_config"` // plugin-specific config

	// Transport tuning
	HTTP2              bool `json:"http2"`                  // enable HTTP/2 (default true for HTTPS)
	DisableKeepAlive   bool `json:"disable_keep_alive"`     // disable keep-alive
	MaxIdleConns       int  `json:"max_idle_conns"`         // max idle connections (default 100)
	MaxIdleConnsPerHost int `json:"max_idle_conns_per_host"` // per-host (default = concurrency)
	DNSCacheEnabled    bool `json:"dns_cache"`              // enable DNS caching

	// Connection warm-up
	WarmupConns int `json:"warmup_conns"` // number of connections to pre-establish (0 = disabled)

	// Adaptive concurrency
	AdaptiveConcurrency bool    `json:"adaptive_concurrency"` // enable adaptive scaling
	AdaptiveTargetMs    float64 `json:"adaptive_target_ms"`   // target P95 latency in ms (default 500)

	// Request batching
	RequestsPerIteration int `json:"requests_per_iteration"` // send N requests per worker iteration (default 1)
}

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func Parse() (*Config, error) {
	var headers headerFlags

	url := flag.String("u", "", "Target URL (required)")
	method := flag.String("m", "GET", "HTTP method (GET, POST, PUT, DELETE, PATCH)")
	body := flag.String("b", "", "Request body")
	concurrency := flag.Int("c", 10, "Number of concurrent workers")
	duration := flag.Duration("d", 10*time.Second, "Test duration (e.g. 10s, 1m)")
	timeout := flag.Duration("t", 30*time.Second, "Request timeout")
	rps := flag.Int("r", 0, "Requests per second limit (0=unlimited)")
	noUI := flag.Bool("no-ui", false, "Disable TUI, print summary only")
	webMode := flag.Bool("web", false, "Start web UI server instead of CLI")
	port := flag.Int("port", 8080, "Web server port (used with --web)")
	cookies := flag.Bool("cookies", false, "Enable per-worker cookie jar for session persistence")
	workerMode := flag.Bool("worker", false, "Run as a distributed worker node")
	workers := flag.String("workers", "", "Comma-separated worker URLs for distributed testing")
	flag.Var(&headers, "H", "HTTP header (e.g. -H 'Content-Type: application/json')")

	flag.Parse()

	var workerURLs []string
	if *workers != "" {
		for _, w := range strings.Split(*workers, ",") {
			w = strings.TrimSpace(w)
			if w != "" {
				workerURLs = append(workerURLs, w)
			}
		}
	}

	if *workerMode {
		return &Config{
			WorkerMode: true,
			Port:       *port,
		}, nil
	}

	if *webMode {
		return &Config{
			WebMode: true,
			Port:    *port,
			Workers: workerURLs,
		}, nil
	}

	if *url == "" {
		return nil, fmt.Errorf("target URL is required (-u)")
	}

	headerMap := make(map[string]string)
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header format: %s (expected Key: Value)", h)
		}
		headerMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return &Config{
		URL:         *url,
		Method:      strings.ToUpper(*method),
		Body:        *body,
		Headers:     headerMap,
		Concurrency: *concurrency,
		Duration:    *duration,
		Timeout:     *timeout,
		RPS:         *rps,
		NoUI:        *noUI,
		CookieJar:   *cookies,
		Workers:     workerURLs,
	}, nil
}
