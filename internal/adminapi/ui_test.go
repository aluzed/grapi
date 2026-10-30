package adminapi

import "testing"

func TestWithBase(t *testing.T) {
	index := []byte(`<html><head><title>x</title></head></html>`)
	if got := string(withBase(index, "/admin/", "")); got != `<html><head><base href="/admin/"><title>x</title></head></html>` {
		t.Errorf("without upload URL: %s", got)
	}
	want := `<html><head><base href="/admin/"><meta name="grapi-upload-url" content="https://cdn.example.com/a&#34;b"><title>x</title></head></html>`
	if got := string(withBase(index, "/admin/", `https://cdn.example.com/a"b`)); got != want {
		t.Errorf("with upload URL: %s", got)
	}
}
