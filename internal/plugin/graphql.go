package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GraphQLPlugin tests GraphQL endpoints with configurable queries.
type GraphQLPlugin struct {
	url       string
	query     string
	variables string // JSON string
	headers   map[string]string
	timeout   time.Duration
	client    *http.Client
}

func (p *GraphQLPlugin) Name() string { return "graphql" }

func (p *GraphQLPlugin) Init(config map[string]string) error {
	p.url = config["url"]
	if p.url == "" {
		return fmt.Errorf("graphql: url is required")
	}
	p.query = config["query"]
	if p.query == "" {
		return fmt.Errorf("graphql: query is required")
	}
	p.variables = config["variables"]
	if p.variables == "" {
		p.variables = "{}"
	}
	p.timeout = 30 * time.Second
	if t, ok := config["timeout"]; ok {
		if d, err := time.ParseDuration(t); err == nil {
			p.timeout = d
		}
	}
	p.headers = make(map[string]string)
	// Parse headers from config (header_xxx = value)
	for k, v := range config {
		if len(k) > 7 && k[:7] == "header_" {
			p.headers[k[7:]] = v
		}
	}
	p.client = &http.Client{Timeout: p.timeout}
	return nil
}

func (p *GraphQLPlugin) Execute(ctx context.Context) RequestResult {
	start := time.Now()

	payload := map[string]interface{}{
		"query": p.query,
	}
	if p.variables != "" && p.variables != "{}" {
		var vars interface{}
		if err := json.Unmarshal([]byte(p.variables), &vars); err == nil {
			payload["variables"] = vars
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", p.url, bytes.NewReader(body))
	if err != nil {
		return RequestResult{StatusCode: 0, Latency: time.Since(start), Error: err}
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return RequestResult{StatusCode: 0, Latency: latency, Error: err}
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Check for GraphQL errors in response
	var gqlResp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(respBody, &gqlResp) == nil && len(gqlResp.Errors) > 0 {
		return RequestResult{
			StatusCode: resp.StatusCode,
			Latency:    latency,
			Error:      fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message),
			Metadata:   map[string]string{"graphql_error": gqlResp.Errors[0].Message},
		}
	}

	return RequestResult{StatusCode: resp.StatusCode, Latency: latency}
}

func (p *GraphQLPlugin) Close() error { return nil }
