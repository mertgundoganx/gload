package prom

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndHandler(t *testing.T) {
	Global.SetRunning(3)
	Global.SetServices(7)
	Global.RecordTestComplete(false, 1000, 50, 250.5, 12.3, 40.1, 5.0)

	rec := httptest.NewRecorder()
	Handler()(rec, httptest.NewRequest("GET", "/metrics", nil))

	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, want := range []string{
		"gload_running_tests 3",
		"gload_total_services 7",
		"gload_tests_completed_total 1",
		"gload_tests_failed_total 1",
		"gload_requests_total 1000",
		"gload_errors_total 50",
		"gload_last_rps 250.50",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, "# TYPE gload_running_tests gauge") {
		t.Error("missing TYPE metadata")
	}
}
