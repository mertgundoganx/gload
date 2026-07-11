package plugin

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

// TCPPlugin tests TCP connectivity (not full HTTP).
type TCPPlugin struct {
	address string
	timeout time.Duration
}

func (p *TCPPlugin) Name() string { return "tcp" }

func (p *TCPPlugin) Init(config map[string]string) error {
	p.address = config["address"] // e.g. "example.com:443"
	p.timeout = 5 * time.Second
	if t, ok := config["timeout"]; ok {
		if d, err := time.ParseDuration(t); err == nil {
			p.timeout = d
		}
	}
	return nil
}

func (p *TCPPlugin) Execute(ctx context.Context) RequestResult {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", p.address, p.timeout)
	latency := time.Since(start)
	if err != nil {
		return RequestResult{StatusCode: 0, Latency: latency, Error: err}
	}
	conn.Close()
	return RequestResult{StatusCode: 200, Latency: latency}
}

func (p *TCPPlugin) Close() error { return nil }

// LatencyHistogramCollector tracks latency distribution in buckets.
type LatencyHistogramCollector struct {
	mu      sync.Mutex
	buckets map[string]int // "0-10ms", "10-50ms", "50-100ms", "100-500ms", "500ms+"
	total   int
	sumMs   float64
	maxMs   float64
}

func (c *LatencyHistogramCollector) Name() string { return "latency_histogram" }

func (c *LatencyHistogramCollector) Collect(result RequestResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buckets == nil {
		c.buckets = make(map[string]int)
	}

	ms := float64(result.Latency.Microseconds()) / 1000
	c.total++
	c.sumMs += ms
	c.maxMs = math.Max(c.maxMs, ms)

	switch {
	case ms < 10:
		c.buckets["0-10ms"]++
	case ms < 50:
		c.buckets["10-50ms"]++
	case ms < 100:
		c.buckets["50-100ms"]++
	case ms < 500:
		c.buckets["100-500ms"]++
	default:
		c.buckets["500ms+"]++
	}
}

func (c *LatencyHistogramCollector) Summary() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]string)
	for k, v := range c.buckets {
		result["bucket_"+k] = fmt.Sprintf("%d", v)
	}
	if c.total > 0 {
		result["avg_ms"] = fmt.Sprintf("%.2f", c.sumMs/float64(c.total))
	}
	result["max_ms"] = fmt.Sprintf("%.2f", c.maxMs)
	return result
}

func (c *LatencyHistogramCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buckets = make(map[string]int)
	c.total = 0
	c.sumMs = 0
	c.maxMs = 0
}

func init() {
	Default.RegisterProtocol("tcp", func() Protocol { return &TCPPlugin{} })
	Default.RegisterProtocol("websocket", func() Protocol { return &WebSocketPlugin{} })
	Default.RegisterProtocol("graphql", func() Protocol { return &GraphQLPlugin{} })
	Default.RegisterProtocol("grpc", func() Protocol { return &GRPCPlugin{} })
	Default.RegisterCollector("latency_histogram", func() MetricCollector { return &LatencyHistogramCollector{} })
}
