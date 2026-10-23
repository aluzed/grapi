package grapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
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
	"github.com/aluzed/grapi/query"
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
	public, _ := app.Query("plugin::users-permissions.role").FindOne(ctx, query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	for _, a := range []string{"upload", "findOne", "destroy"} {
		if _, err := app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api." + a, "role": roleID}, query.Params{}); err != nil {
			t.Fatal(err)
		}
	}
	var events []string
	app.Events().Subscribe(func(name string, _ map[string]any) { events = append(events, name) })
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

// Past sharp.limitInputPixels Strapi's sharp calls throw and the upload
// stores a plain file: no dimensions, no formats.
func TestUploadLimitInputPixels(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeLimit := func(v string) {
		conf := "upload:\n  config:\n    sharp:\n      limitInputPixels: " + v + "\n"
		if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	writeLimit("-1")
	if _, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup(); err == nil || !strings.Contains(err.Error(), `"sharp.limitInputPixels" must be a boolean or an integer`) {
		t.Fatalf("an invalid limit stops the start: %v", err)
	}
	writeLimit("300")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ctx := context.Background()
	public, _ := app.Query("plugin::users-permissions.role").FindOne(ctx, query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	if _, err := app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api.upload", "role": roleID}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	upload := func(w, h int) map[string]any {
		body, ct := pngBody(t, w, h, "")
		res, err := http.Post(ts.URL+"/api/upload", ct, body)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var out []map[string]any
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil || len(out) != 1 {
			t.Fatalf("upload %dx%d: %d %v", w, h, res.StatusCode, err)
		}
		return out[0]
	}
	if f := upload(10, 10); f["width"] != float64(10) {
		t.Fatalf("under the limit: %v", f)
	}
	if f := upload(20, 20); f["width"] != nil || f["height"] != nil || f["formats"] != nil || f["mime"] != "image/png" {
		t.Fatalf("over the limit, a plain file: %v", f)
	}
}

func TestUploadSizeLimitLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("upload:\n  config:\n    sizeLimit: 2000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
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
	public, _ := app.Query("plugin::users-permissions.role").FindOne(ctx, query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	if _, err := app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api.upload", "role": roleID}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	// Random pixels do not compress: well over 2000 bytes.
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, _ := mw.CreateFormFile("files", "noise.png")
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	_, _ = rand.Read(img.Pix)
	_ = png.Encode(part, img)
	_ = mw.Close()
	res, err := http.Post(ts.URL+"/api/upload", mw.FormDataContentType(), &b)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	_ = res.Body.Close()
	e2, _ := body["error"].(map[string]any)
	if res.StatusCode != 413 || e2["name"] != "PayloadTooLargeError" || !strings.Contains(fmt.Sprint(e2["message"]), "noise.png exceeds size limit of 2 KB") {
		t.Fatalf("over sizeLimit: %d %v", res.StatusCode, body)
	}
	left, _ := os.ReadDir(tmp)
	for _, f := range left {
		if strings.HasPrefix(f.Name(), "grapi-upload-") {
			t.Fatalf("a temporary upload directory is left: %s", f.Name())
		}
	}
}
