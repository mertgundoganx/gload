package plugin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// WebSocketPlugin tests WebSocket endpoints with configurable message patterns.
type WebSocketPlugin struct {
	url            string
	message        string // message to send on each Execute
	expectResponse bool
	timeout        time.Duration

	mu   sync.Mutex
	conn *websocket.Conn
}

func (p *WebSocketPlugin) Name() string { return "websocket" }

func (p *WebSocketPlugin) Init(config map[string]string) error {
	p.url = config["url"]
	if p.url == "" {
		return fmt.Errorf("websocket: url is required")
	}
	p.message = config["message"]
	if p.message == "" {
		p.message = "ping"
	}
	p.expectResponse = config["expect_response"] != "false"
	p.timeout = 10 * time.Second
	if t, ok := config["timeout"]; ok {
		if d, err := time.ParseDuration(t); err == nil {
			p.timeout = d
		}
	}
	return nil
}

func (p *WebSocketPlugin) Execute(ctx context.Context) RequestResult {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	start := time.Now()

	// Connect (or reuse connection)
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		var err error
		conn, _, err = websocket.Dial(ctx, p.url, nil)
		if err != nil {
			return RequestResult{StatusCode: 0, Latency: time.Since(start), Error: err}
		}
		p.mu.Lock()
		p.conn = conn
		p.mu.Unlock()
	}

	// Send message
	err := conn.Write(ctx, websocket.MessageText, []byte(p.message))
	if err != nil {
		// Connection broken, reset
		p.mu.Lock()
		p.conn = nil
		p.mu.Unlock()
		return RequestResult{StatusCode: 0, Latency: time.Since(start), Error: err}
	}

	// Wait for response if expected
	if p.expectResponse {
		_, _, err = conn.Read(ctx)
		if err != nil {
			p.mu.Lock()
			p.conn = nil
			p.mu.Unlock()
			return RequestResult{StatusCode: 0, Latency: time.Since(start), Error: err}
		}
	}

	return RequestResult{StatusCode: 200, Latency: time.Since(start)}
}

func (p *WebSocketPlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Close(websocket.StatusNormalClosure, "done")
		p.conn = nil
	}
	return nil
}
