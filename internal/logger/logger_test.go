package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// restore returns the Default logger to a known state after a test mutates it.
func restore(t *testing.T) {
	t.Helper()
	prevLevel, prevJSON, prevStatic, prevOut := Default.level, Default.json, Default.static, Default.output
	t.Cleanup(func() {
		Default.level = prevLevel
		Default.json = prevJSON
		Default.static = prevStatic
		Default.output = prevOut
	})
}

func TestLevelString(t *testing.T) {
	cases := map[Level]string{DEBUG: "debug", INFO: "info", WARN: "warn", ERROR: "error", Level(99): "unknown"}
	for l, want := range cases {
		if got := l.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", l, got, want)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	restore(t)
	var buf bytes.Buffer
	SetOutput(&buf)
	SetJSON(false)
	SetLevel(WARN)

	Debug("dbg")
	Info("inf")
	Warn("wrn")
	Error("err")

	out := buf.String()
	if strings.Contains(out, "dbg") || strings.Contains(out, "inf") {
		t.Errorf("debug/info should be filtered at WARN level:\n%s", out)
	}
	if !strings.Contains(out, "wrn") || !strings.Contains(out, "err") {
		t.Errorf("warn/error should be present:\n%s", out)
	}
}

func TestJSONOutput(t *testing.T) {
	restore(t)
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel(DEBUG)
	SetJSON(true)
	SetStatic("service", "gload")

	Info("hello", Fields("count", 3, "ok", true))

	line := strings.TrimSpace(buf.String())
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, line)
	}
	if entry["level"] != "info" || entry["msg"] != "hello" || entry["service"] != "gload" {
		t.Errorf("unexpected entry: %v", entry)
	}
	if entry["count"].(float64) != 3 || entry["ok"] != true {
		t.Errorf("fields missing/wrong: %v", entry)
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("timestamp missing")
	}
}

func TestJSONCallerOnWarn(t *testing.T) {
	restore(t)
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel(DEBUG)
	SetJSON(true)

	Warn("careful")
	var entry map[string]interface{}
	json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry)
	if _, ok := entry["caller"]; !ok {
		t.Error("warn entry should include caller info")
	}
}

func TestFieldsHelpers(t *testing.T) {
	if m := F("k", "v"); m["k"] != "v" {
		t.Error("F failed")
	}
	m := Fields("a", 1, "b", "two", "dangling")
	if m["a"].(int) != 1 || m["b"] != "two" {
		t.Errorf("Fields parsed wrong: %v", m)
	}
	if _, ok := m["dangling"]; ok {
		t.Error("odd trailing key should be ignored")
	}
	if mergeFields(nil) != nil {
		t.Error("mergeFields(nil) should be nil")
	}
}
