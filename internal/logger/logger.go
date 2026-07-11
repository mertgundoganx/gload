package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "debug"
	case INFO:
		return "info"
	case WARN:
		return "warn"
	case ERROR:
		return "error"
	default:
		return "unknown"
	}
}

// Logger is a simple structured logger with optional JSON output.
type Logger struct {
	mu     sync.Mutex
	level  Level
	output io.Writer
	json   bool
	// Static fields appended to every JSON entry (e.g. "service", "hostname").
	static map[string]interface{}
}

var Default = &Logger{level: INFO, output: os.Stderr, json: false}

func SetLevel(l Level)      { Default.level = l }
func SetJSON(enabled bool)  { Default.json = enabled }
func SetOutput(w io.Writer) { Default.output = w }

// SetStatic adds a static key-value pair included in every JSON log entry.
// Useful for ELK/Loki: service name, hostname, version, etc.
func SetStatic(key string, val interface{}) {
	if Default.static == nil {
		Default.static = make(map[string]interface{})
	}
	Default.static[key] = val
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.json {
		entry := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"level":     level.String(),
			"msg":       msg,
		}
		// Append static fields (service, hostname, version, pid).
		for k, v := range l.static {
			entry[k] = v
		}
		// Append caller info for warn/error levels.
		if level >= WARN {
			if _, file, line, ok := runtime.Caller(2); ok {
				entry["caller"] = fmt.Sprintf("%s:%d", file, line)
			}
		}
		for k, v := range fields {
			entry[k] = v
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.output, string(data))
	} else {
		prefix := fmt.Sprintf("%s [%s] ", time.Now().Format("15:04:05"), level.String())
		if len(fields) > 0 {
			fmt.Fprintf(l.output, "%s%s %v\n", prefix, msg, fields)
		} else {
			fmt.Fprintf(l.output, "%s%s\n", prefix, msg)
		}
	}
}

func Debug(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields)
	Default.log(DEBUG, msg, f)
}

func Info(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields)
	Default.log(INFO, msg, f)
}

func Warn(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields)
	Default.log(WARN, msg, f)
}

func Error(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields)
	Default.log(ERROR, msg, f)
}

func mergeFields(fields []map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	return fields[0]
}

// F is a shortcut for creating field maps
func F(key string, val interface{}) map[string]interface{} {
	return map[string]interface{}{key: val}
}

// Fields merges multiple field maps
func Fields(pairs ...interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for i := 0; i < len(pairs)-1; i += 2 {
		if key, ok := pairs[i].(string); ok {
			m[key] = pairs[i+1]
		}
	}
	return m
}
