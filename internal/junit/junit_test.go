package junit

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestGenerateFromAssertions(t *testing.T) {
	t.Parallel()

	assertions := []AssertionResult{
		{Metric: "p95_latency", Operator: "lt", Value: 200, Actual: 150, Passed: true},
		{Metric: "error_rate", Operator: "lt", Value: 5, Actual: 8.5, Passed: false},
	}

	var buf bytes.Buffer
	err := GenerateFromAssertions(&buf, "Test Service", 30*time.Second, assertions)
	if err != nil {
		t.Fatalf("GenerateFromAssertions failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "<?xml") {
		t.Error("missing XML header")
	}
	if !strings.Contains(output, "Test Service") {
		t.Error("missing service name")
	}

	// Parse XML to verify structure
	var suites TestSuites
	if err := xml.Unmarshal(buf.Bytes(), &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites.Suites))
	}
	suite := suites.Suites[0]
	if suite.Tests != 2 {
		t.Errorf("expected 2 tests, got %d", suite.Tests)
	}
	if suite.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suite.Failures)
	}
	if suite.Cases[0].Failure != nil {
		t.Error("first case should pass")
	}
	if suite.Cases[1].Failure == nil {
		t.Error("second case should fail")
	}
}

func TestGenerateEmptyAssertions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := GenerateFromAssertions(&buf, "Empty", 10*time.Second, []AssertionResult{})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("empty output")
	}
}

func TestGenerateAllPassing(t *testing.T) {
	t.Parallel()

	assertions := []AssertionResult{
		{Metric: "rps", Operator: "gt", Value: 100, Actual: 150, Passed: true},
		{Metric: "p95_latency", Operator: "lt", Value: 500, Actual: 200, Passed: true},
	}

	var buf bytes.Buffer
	GenerateFromAssertions(&buf, "All Pass", 10*time.Second, assertions)

	var suites TestSuites
	xml.Unmarshal(buf.Bytes(), &suites)
	if suites.Suites[0].Failures != 0 {
		t.Error("expected 0 failures")
	}
}
