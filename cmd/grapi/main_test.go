package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	version = "1.2.3"
	tests := []struct {
		args       []string
		code       int
		stdout     string
		stderrPart string
	}{
		{[]string{"version"}, 0, "1.2.3\n", ""},
		{[]string{"--version"}, 0, "1.2.3\n", ""},
		{nil, 2, "", "usage"},
		{[]string{"nope"}, 2, "", "unknown command"},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		if code := run(tt.args, &out, &errOut); code != tt.code {
			t.Errorf("run(%v) = %d, want %d", tt.args, code, tt.code)
		}
		if out.String() != tt.stdout {
			t.Errorf("run(%v) stdout = %q, want %q", tt.args, out.String(), tt.stdout)
		}
		if !bytes.Contains(errOut.Bytes(), []byte(tt.stderrPart)) {
			t.Errorf("run(%v) stderr = %q, want it to contain %q", tt.args, errOut.String(), tt.stderrPart)
		}
	}
}
