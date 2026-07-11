package junit

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

type TestSuites struct {
	XMLName xml.Name    `xml:"testsuites"`
	Suites  []TestSuite `xml:"testsuite"`
}

type TestSuite struct {
	XMLName   xml.Name   `xml:"testsuite"`
	Name      string     `xml:"name,attr"`
	Tests     int        `xml:"tests,attr"`
	Failures  int        `xml:"failures,attr"`
	Errors    int        `xml:"errors,attr"`
	Time      float64    `xml:"time,attr"`
	Timestamp string     `xml:"timestamp,attr"`
	Cases     []TestCase `xml:"testcase"`
}

type TestCase struct {
	XMLName xml.Name `xml:"testcase"`
	Name    string   `xml:"name,attr"`
	Time    float64  `xml:"time,attr"`
	Failure *Failure `xml:"failure,omitempty"`
}

type Failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

// GenerateFromAssertions creates a JUnit XML report from test assertions.
func GenerateFromAssertions(w io.Writer, serviceName string, duration time.Duration, assertions []AssertionResult) error {
	suite := TestSuite{
		Name:      serviceName,
		Tests:     len(assertions),
		Time:      duration.Seconds(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	for _, a := range assertions {
		tc := TestCase{
			Name: fmt.Sprintf("%s %s %v", a.Metric, a.Operator, a.Value),
			Time: duration.Seconds(),
		}
		if !a.Passed {
			suite.Failures++
			tc.Failure = &Failure{
				Message: fmt.Sprintf("Expected %s %s %v but got %v", a.Metric, a.Operator, a.Value, a.Actual),
				Type:    "AssertionFailure",
				Text:    fmt.Sprintf("Metric: %s\nOperator: %s\nExpected: %v\nActual: %v", a.Metric, a.Operator, a.Value, a.Actual),
			}
		}
		suite.Cases = append(suite.Cases, tc)
	}

	suites := TestSuites{Suites: []TestSuite{suite}}

	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(suites)
}

type AssertionResult struct {
	Metric   string
	Operator string
	Value    float64
	Actual   float64
	Passed   bool
}
