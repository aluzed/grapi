package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/query"
)

func pngBody(t *testing.T, w, h int, fileInfo string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	if fileInfo != "" {
		_ = mw.WriteField("fileInfo", fileInfo)
	}
	part, err := mw.CreateFormFile("files", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(part, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

func TestUploadReplaceUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ctx := context.Background()
	e := app.Documents().E
	public, _ := e.FindOne(ctx, "plugin::users-permissions.role", query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	for _, a := range []string{"upload", "findOne", "destroy"} {
		if _, err := e.Create(ctx, "plugin::users-permissions.permission", query.Data{"action": "plugin::upload.content-api." + a, "role": roleID}, query.Params{}); err != nil {
			t.Fatal(err)
		}
	}
	var events []string
	app.Documents().Events.Subscribe(func(name string, _ map[string]any) { events = append(events, name) })
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	send := func(method, url, ct string, body *bytes.Buffer) (int, any) {
		req, _ := http.NewRequest(method, ts.URL+url, body)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var out any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	b, ct := pngBody(t, 800, 600, `{"alternativeText":"first"}`)
	code, out := send("POST", "/api/upload", ct, b)
	if code != 201 {
		t.Fatalf("upload %d %v", code, out)
	}
	file := out.([]any)[0].(map[string]any)
	id := file["id"].(float64)
	url := file["url"].(string)
	b, ct = pngBody(t, 400, 300, "")
	code, out = send("POST", "/api/upload?id="+jsonNum(id), ct, b)
	if code != 200 {
		t.Fatalf("replace %d %v", code, out)
	}
	replaced := out.(map[string]any)
	if replaced["url"] != url || replaced["width"] != float64(400) || replaced["alternativeText"] != "first" {
		t.Fatalf("replace kept url %v (want %v), width %v, alt %v", replaced["url"], url, replaced["width"], replaced["alternativeText"])
	}
	formats := replaced["formats"].(map[string]any)
	if _, ok := formats["small"]; ok {
		t.Fatalf("400x300 must not have a small format: %v", formats)
	}
	code, out = send("POST", "/api/upload?id="+jsonNum(id), "application/json", bytes.NewBufferString(`{"fileInfo":{"name":"renamed.png","caption":"c"}}`))
	if code != 200 || out.(map[string]any)["name"] != "renamed.png" || out.(map[string]any)["caption"] != "c" {
		t.Fatalf("update info %d %v", code, out)
	}
	uploads := filepath.Join(dir, "public", "uploads")
	before, _ := os.ReadDir(uploads)
	if code, out = send("DELETE", "/api/upload/files/"+jsonNum(id), "", bytes.NewBuffer(nil)); code != 200 {
		t.Fatalf("delete %d %v", code, out)
	}
	after, _ := os.ReadDir(uploads)
	if len(before) == 0 || len(after) != 0 {
		t.Fatalf("files on disk: %d before, %d after", len(before), len(after))
	}
	if strings.Join(events, ",") != "media.create,media.update,media.update,media.delete" {
		t.Fatalf("events %v", events)
	}
}

func jsonNum(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
