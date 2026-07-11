package github

import (
	"os"
	"strings"
	"testing"
)

func TestFormatComment(t *testing.T) {
	pass := FormatComment(TestResult{
		ServiceName: "Checkout API", Status: "pass",
		RPS: 1234.5, AvgLatency: 12.3, P95Latency: 40, P99Latency: 60,
		ErrorRate: 0.5, TotalReqs: 10000, DurationMs: 30000,
		Assertions: []AssertionResult{
			{Metric: "p95_latency", Operator: "lt", Value: 500, Actual: 40, Passed: true},
		},
	})
	for _, want := range []string{"Checkout API", "PASSED", "1234.5 /s", "10000", "Assertions", "p95_latency"} {
		if !strings.Contains(pass, want) {
			t.Errorf("comment missing %q", want)
		}
	}

	fail := FormatComment(TestResult{ServiceName: "X", Status: "fail", RPS: 1})
	if !strings.Contains(fail, "FAILED") {
		t.Error("failed status not rendered")
	}
}

func TestFormatValue(t *testing.T) {
	if got := formatValue(40, "p95_latency"); got != "40.0ms" {
		t.Errorf("latency = %q", got)
	}
	if got := formatValue(5, "error_rate"); got != "5.0%" {
		t.Errorf("error_rate = %q", got)
	}
	if got := formatValue(100, "rps"); got != "100.0" {
		t.Errorf("rps = %q", got)
	}
}

func TestPostPRCommentMissingEnv(t *testing.T) {
	for _, k := range []string{"GITHUB_TOKEN", "GITHUB_REPOSITORY", "GLOAD_PR_NUMBER", "GITHUB_REF"} {
		old := os.Getenv(k)
		os.Unsetenv(k)
		t.Cleanup(func() { os.Setenv(k, old) })
	}
	if err := PostPRComment(TestResult{ServiceName: "x", Status: "pass"}); err == nil {
		t.Error("expected error when GitHub env vars are missing")
	}
}
