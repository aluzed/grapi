package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestConfigurationDumpRestore(t *testing.T) {
	dir := filepath.Join("..", "..", "compat", "fixtures", "basic")
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(t.TempDir(), "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	var out bytes.Buffer
	if err := configurationDump([]string{"-dir", dir}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	var advanced map[string]any
	for _, e := range entries {
		if !strings.HasPrefix(e["key"].(string), "plugin_") {
			t.Errorf("only plugin_ keys: %v", e["key"])
		}
		if e["key"] == "plugin_users-permissions_advanced" {
			advanced = e
		}
	}
	if advanced == nil || !strings.Contains(advanced["value"].(string), `"allow_register":true`) {
		t.Fatalf("advanced settings: %v", advanced)
	}
	advanced["value"] = strings.Replace(advanced["value"].(string), `"allow_register":true`, `"allow_register":false`, 1)
	file := filepath.Join(t.TempDir(), "dump.json")
	b, _ := json.Marshal([]any{advanced, map[string]any{"key": "plugin_demo_settings", "value": `{"a":1}`, "type": "object", "environment": nil, "tag": nil}})
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := configurationRestore([]string{"-dir", dir, "-f", file, "-s", "keep"}, nil, &out, io.Discard); err != nil || !strings.Contains(out.String(), "1 created, 1 untouched") {
		t.Fatalf("keep: %v %s", err, out.String())
	}
	out.Reset()
	if err := configurationRestore([]string{"-dir", dir, "-f", file}, nil, &out, io.Discard); err != nil || !strings.Contains(out.String(), "0 created, 2 replaced") {
		t.Fatalf("replace: %v %s", err, out.String())
	}
	out.Reset()
	if err := configurationDump([]string{"-dir", dir}, &out, io.Discard); err != nil || !strings.Contains(out.String(), `\"allow_register\":false`) || !strings.Contains(out.String(), "plugin_demo_settings") {
		t.Fatalf("after replace: %v %s", err, out.String())
	}
}
