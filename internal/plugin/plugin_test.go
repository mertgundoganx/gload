package plugin

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func newRegistry() *Registry {
	return &Registry{
		protocols:  make(map[string]func() Protocol),
		collectors: make(map[string]func() MetricCollector),
	}
}

func TestRegistry(t *testing.T) {
	r := newRegistry()
	r.RegisterProtocol("tcp", func() Protocol { return &TCPPlugin{} })

	p, ok := r.NewProtocol("tcp")
	if !ok || p == nil || p.Name() != "tcp" {
		t.Fatalf("NewProtocol(tcp) = %v, %v", p, ok)
	}
	if _, ok := r.NewProtocol("missing"); ok {
		t.Error("NewProtocol(missing) should be false")
	}
	if list := r.ListProtocols(); len(list) != 1 || list[0] != "tcp" {
		t.Errorf("ListProtocols = %v", list)
	}

	r.RegisterCollector("hist", func() MetricCollector { return &LatencyHistogramCollector{} })
	if c, ok := r.NewCollector("hist"); !ok || c == nil {
		t.Error("NewCollector(hist) failed")
	}
	if _, ok := r.NewCollector("missing"); ok {
		t.Error("NewCollector(missing) should be false")
	}
	if list := r.ListCollectors(); len(list) != 1 {
		t.Errorf("ListCollectors = %v", list)
	}
}

func TestDefaultRegistryHasBuiltins(t *testing.T) {
	protos := Default.ListProtocols()
	for _, want := range []string{"tcp", "websocket", "graphql", "grpc"} {
		found := false
		for _, p := range protos {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("built-in protocol %q not registered (have %v)", want, protos)
		}
	}
	if _, ok := Default.NewCollector("latency_histogram"); !ok {
		t.Error("latency_histogram collector not registered")
	}
}

func TestTCPPlugin(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	p := &TCPPlugin{}
	if err := p.Init(map[string]string{"address": ln.Addr().String(), "timeout": "2s"}); err != nil {
		t.Fatal(err)
	}
	if res := p.Execute(context.Background()); res.StatusCode != 200 || res.Error != nil {
		t.Errorf("Execute on live listener = %+v", res)
	}
	if err := p.Close(); err != nil {
		t.Error(err)
	}

	// Unreachable address → error, status 0.
	bad := &TCPPlugin{}
	bad.Init(map[string]string{"address": "127.0.0.1:1"})
	if res := bad.Execute(context.Background()); res.StatusCode != 0 || res.Error == nil {
		t.Errorf("Execute on dead address = %+v (want status 0 + error)", res)
	}
}

func TestLatencyHistogramCollector(t *testing.T) {
	c := &LatencyHistogramCollector{}
	for i := 0; i < 5; i++ {
		c.Collect(RequestResult{StatusCode: 200, Latency: 5 * time.Millisecond})
	}
	sum := c.Summary()
	if sum["avg_ms"] == "" {
		t.Error("Summary should report avg_ms after collecting")
	}
	if sum["bucket_0-10ms"] != "5" {
		t.Errorf("expected 5 in the 0-10ms bucket, got %q", sum["bucket_0-10ms"])
	}

	c.Reset()
	after := c.Summary()
	if _, ok := after["avg_ms"]; ok {
		t.Error("avg_ms should be gone after Reset")
	}
	for k := range after {
		if strings.HasPrefix(k, "bucket_") {
			t.Errorf("buckets should be cleared after Reset, found %q", k)
		}
	}
	if c.Name() != "latency_histogram" {
		t.Errorf("Name = %q", c.Name())
	}
}
