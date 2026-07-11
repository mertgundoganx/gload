package config

import (
	"flag"
	"io"
	"os"
	"testing"
	"time"
)

// parseArgs runs Parse() with a fresh flag set and the given CLI args, then
// restores global flag/os state.
func parseArgs(t *testing.T, args ...string) (*Config, error) {
	t.Helper()
	oldArgs, oldCmd := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = oldArgs, oldCmd })

	flag.CommandLine = flag.NewFlagSet("gload", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"gload"}, args...)
	return Parse()
}

func TestParseBasic(t *testing.T) {
	cfg, err := parseArgs(t, "-u", "https://api.example.com", "-c", "50", "-d", "30s", "-r", "200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "https://api.example.com" {
		t.Errorf("URL = %q", cfg.URL)
	}
	if cfg.Concurrency != 50 {
		t.Errorf("Concurrency = %d, want 50", cfg.Concurrency)
	}
	if cfg.Duration != 30*time.Second {
		t.Errorf("Duration = %v", cfg.Duration)
	}
	if cfg.RPS != 200 {
		t.Errorf("RPS = %d, want 200", cfg.RPS)
	}
	if cfg.Method != "GET" {
		t.Errorf("default Method = %q, want GET", cfg.Method)
	}
}

func TestParseMethodUppercased(t *testing.T) {
	cfg, err := parseArgs(t, "-u", "https://x.com", "-m", "post")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Method != "POST" {
		t.Errorf("Method = %q, want POST", cfg.Method)
	}
}

func TestParseHeaders(t *testing.T) {
	cfg, err := parseArgs(t, "-u", "https://x.com",
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer tok")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header = %q", cfg.Headers["Content-Type"])
	}
	if cfg.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("Authorization header = %q", cfg.Headers["Authorization"])
	}
}

func TestParseInvalidHeader(t *testing.T) {
	_, err := parseArgs(t, "-u", "https://x.com", "-H", "no-colon-here")
	if err == nil {
		t.Fatal("expected error for malformed header")
	}
}

func TestParseMissingURL(t *testing.T) {
	_, err := parseArgs(t)
	if err == nil {
		t.Fatal("expected error when -u is missing")
	}
}

func TestParseWebMode(t *testing.T) {
	cfg, err := parseArgs(t, "--web", "--port", "9090")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WebMode || cfg.Port != 9090 {
		t.Errorf("WebMode=%v Port=%d, want true/9090", cfg.WebMode, cfg.Port)
	}
}

func TestParseWorkerMode(t *testing.T) {
	cfg, err := parseArgs(t, "--worker", "--port", "8081")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WorkerMode || cfg.Port != 8081 {
		t.Errorf("WorkerMode=%v Port=%d, want true/8081", cfg.WorkerMode, cfg.Port)
	}
}

func TestParseWorkersList(t *testing.T) {
	cfg, err := parseArgs(t, "--web", "--workers", "http://a:1 , http://b:2,")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workers) != 2 || cfg.Workers[0] != "http://a:1" || cfg.Workers[1] != "http://b:2" {
		t.Errorf("Workers = %#v, want [http://a:1 http://b:2] (trimmed, empty dropped)", cfg.Workers)
	}
}

func TestParseFlags(t *testing.T) {
	cfg, err := parseArgs(t, "-u", "https://x.com", "--no-ui", "--cookies", "-b", `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NoUI || !cfg.CookieJar || cfg.Body != `{"a":1}` {
		t.Errorf("flags not applied: %+v", cfg)
	}
}

func TestHeaderFlags(t *testing.T) {
	var h headerFlags
	if err := h.Set("A: 1"); err != nil {
		t.Fatal(err)
	}
	h.Set("B: 2")
	if h.String() != "A: 1, B: 2" {
		t.Errorf("String() = %q", h.String())
	}
}
