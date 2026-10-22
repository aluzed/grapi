package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLevels(t *testing.T) {
	var b bytes.Buffer
	log := New(&b, Text, DefaultLevel)
	log.Debug("hidden")
	log.Log(context.Background(), LevelHTTP, "GET / (1 ms) 200")
	log.Info("shown")
	out := b.String()
	if strings.Contains(out, "hidden") || !strings.Contains(out, "level=HTTP") || !strings.Contains(out, "shown") {
		t.Fatal(out)
	}
	for in, want := range map[string]slog.Level{"error": slog.LevelError, "warn": slog.LevelWarn, "info": slog.LevelInfo, "http": LevelHTTP, "debug": slog.LevelDebug, "": LevelHTTP} {
		if got, err := ParseLevel(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Error("loud is not a level")
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	New(&b, JSON, DefaultLevel).Log(context.Background(), LevelHTTP, "x", "request_id", "abc")
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil || m["level"] != "HTTP" || m["request_id"] != "abc" {
		t.Fatalf("%s %v", b.String(), err)
	}
	if DefaultFormat("production") != JSON || DefaultFormat("development") != Text {
		t.Fatal("default format")
	}
}
