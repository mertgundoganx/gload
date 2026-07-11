package plugin

import (
	"context"
	"sort"
	"sync"
	"time"
)

// RequestResult contains the outcome of a single plugin request.
type RequestResult struct {
	StatusCode int
	Latency    time.Duration
	Error      error
	Metadata   map[string]string // custom key-value pairs from plugin
}

// Protocol defines the interface for custom protocol plugins.
// Implementations handle non-HTTP protocols (gRPC, WebSocket, TCP, etc.)
type Protocol interface {
	// Name returns the protocol name (e.g. "grpc", "websocket", "tcp")
	Name() string

	// Init is called once before tests start with the service config.
	Init(config map[string]string) error

	// Execute performs a single request/operation.
	Execute(ctx context.Context) RequestResult

	// Close cleans up resources.
	Close() error
}

// MetricCollector defines the interface for custom metric plugins.
// Implementations can collect additional metrics during tests.
type MetricCollector interface {
	// Name returns the collector name.
	Name() string

	// Collect is called after each request with the result.
	Collect(result RequestResult)

	// Summary returns collected metrics as key-value pairs.
	Summary() map[string]string

	// Reset clears collected data.
	Reset()
}

// Registry holds registered plugin factories. Factories (rather than shared
// instances) let each test run obtain its own protocol instance, since protocol
// plugins hold per-run connection state that cannot be shared across concurrent
// runners. This registry is the single source of truth for both the runner
// (which instantiates protocols) and the /api/plugins listing.
type Registry struct {
	mu         sync.RWMutex
	protocols  map[string]func() Protocol
	collectors map[string]func() MetricCollector
}

// Default is the global plugin registry.
var Default = &Registry{
	protocols:  make(map[string]func() Protocol),
	collectors: make(map[string]func() MetricCollector),
}

// RegisterProtocol registers a protocol plugin factory under the given name.
func (r *Registry) RegisterProtocol(name string, factory func() Protocol) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protocols[name] = factory
}

// RegisterCollector registers a metric collector plugin factory under the given name.
func (r *Registry) RegisterCollector(name string, factory func() MetricCollector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors[name] = factory
}

// NewProtocol creates a fresh instance of the named protocol plugin.
func (r *Registry) NewProtocol(name string) (Protocol, bool) {
	r.mu.RLock()
	factory, ok := r.protocols[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(), true
}

// NewCollector creates a fresh instance of the named collector plugin.
func (r *Registry) NewCollector(name string) (MetricCollector, bool) {
	r.mu.RLock()
	factory, ok := r.collectors[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(), true
}

// ListProtocols returns the names of all registered protocols, sorted so the
// order is stable across calls.
func (r *Registry) ListProtocols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.protocols))
	for name := range r.protocols {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListCollectors returns the names of all registered collectors, sorted.
func (r *Registry) ListCollectors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.collectors))
	for name := range r.collectors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
