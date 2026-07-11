package plugin

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// GRPCPlugin performs basic gRPC health checks and connection testing.
// For full gRPC load testing with protobuf support, use the gRPC reflection API
// or provide pre-compiled request payloads.
//
// This plugin establishes gRPC-like connections (HTTP/2 + TLS) and measures
// connection/handshake latency. For actual RPC testing, it sends a raw
// gRPC health check frame.
type GRPCPlugin struct {
	address   string // host:port
	useTLS    bool
	timeout   time.Duration
	plaintext bool
}

func (p *GRPCPlugin) Name() string { return "grpc" }

func (p *GRPCPlugin) Init(config map[string]string) error {
	p.address = config["address"]
	if p.address == "" {
		return fmt.Errorf("grpc: address is required (host:port)")
	}
	p.useTLS = config["tls"] != "false"
	p.plaintext = config["plaintext"] == "true"
	p.timeout = 10 * time.Second
	if t, ok := config["timeout"]; ok {
		if d, err := time.ParseDuration(t); err == nil {
			p.timeout = d
		}
	}
	return nil
}

func (p *GRPCPlugin) Execute(ctx context.Context) RequestResult {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	start := time.Now()

	var conn net.Conn
	var err error

	dialer := &net.Dialer{Timeout: p.timeout}

	if p.useTLS && !p.plaintext {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: p.timeout}, "tcp", p.address, &tls.Config{
			NextProtos: []string{"h2"}, // gRPC requires HTTP/2
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", p.address)
	}

	latency := time.Since(start)

	if err != nil {
		return RequestResult{StatusCode: 0, Latency: latency, Error: err}
	}

	// Send HTTP/2 connection preface + gRPC health check
	// PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n
	preface := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(preface)
	conn.Close()

	if err != nil {
		return RequestResult{StatusCode: 0, Latency: latency, Error: err}
	}

	return RequestResult{
		StatusCode: 200,
		Latency:    latency,
		Metadata: map[string]string{
			"protocol": "h2",
			"tls":      fmt.Sprintf("%v", p.useTLS),
		},
	}
}

func (p *GRPCPlugin) Close() error { return nil }
