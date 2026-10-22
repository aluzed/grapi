package events

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestHub(t *testing.T) {
	var logs bytes.Buffer
	h := New(slog.New(slog.NewTextHandler(&logs, nil)))
	var got []string
	h.On("entry.create", func(map[string]any) { panic("boom") })
	off := h.On("entry.create", func(p map[string]any) { got = append(got, "on:"+p["uid"].(string)) })
	h.Once("entry.create", func(map[string]any) { got = append(got, "once") })
	h.Subscribe(func(name string, _ map[string]any) { got = append(got, "all:"+name) })
	h.Emit("entry.create", map[string]any{"uid": "api::a.a"})
	off()
	h.Emit("entry.create", map[string]any{"uid": "api::a.a"})
	want := []string{"on:api::a.a", "once", "all:entry.create", "all:entry.create"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
	if !bytes.Contains(logs.Bytes(), []byte("event listener panicked")) {
		t.Fatal("the panic must be logged")
	}
}
