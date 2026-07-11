package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendWebhook(t *testing.T) {
	t.Parallel()

	var received TestResult
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type")
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	result := TestResult{
		ServiceName: "Test Service",
		Status:      "pass",
		RPS:         100.5,
		AvgLatency:  50.2,
		TotalReqs:   1000,
	}

	err := SendWebhook(srv.URL, result)
	if err != nil {
		t.Fatalf("SendWebhook failed: %v", err)
	}
	if received.ServiceName != "Test Service" {
		t.Errorf("wrong name: %s", received.ServiceName)
	}
	if received.RPS != 100.5 {
		t.Errorf("wrong RPS: %f", received.RPS)
	}
}

func TestSendWebhookError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	err := SendWebhook(srv.URL, TestResult{})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSendSlack(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := SendSlack(srv.URL, TestResult{ServiceName: "API", Status: "fail", RPS: 50})
	if err != nil {
		t.Fatalf("SendSlack failed: %v", err)
	}
	if receivedBody["attachments"] == nil {
		t.Error("missing attachments")
	}
}

func TestSendTeams(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := SendTeams(srv.URL, TestResult{ServiceName: "API", Status: "pass"})
	if err != nil {
		t.Fatalf("SendTeams failed: %v", err)
	}
	if receivedBody["@type"] != "MessageCard" {
		t.Error("wrong type")
	}
}

func TestSendDiscord(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := SendDiscord(srv.URL, TestResult{ServiceName: "API", Status: "fail"})
	if err != nil {
		t.Fatalf("SendDiscord failed: %v", err)
	}
	if receivedBody["embeds"] == nil {
		t.Error("missing embeds")
	}
}

func TestNotifyAllChannels(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	Notify(srv.URL, srv.URL, srv.URL, srv.URL, TestResult{ServiceName: "Test"})
	if calls != 4 {
		t.Errorf("expected 4 calls, got %d", calls)
	}
}

func TestNotifySkipsEmpty(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	Notify("", srv.URL, "", "", TestResult{})
	if calls != 1 {
		t.Errorf("expected 1 call (slack only), got %d", calls)
	}
}
