//go:build integration

package grapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/query"
)

// Needs GRAPI_TEST_S3_ENDPOINT (an S3 compatible store with bucket
// "grapi", credentials GRAPI_TEST_S3_KEY / GRAPI_TEST_S3_SECRET).
func TestUploadToS3(t *testing.T) {
	endpoint := os.Getenv("GRAPI_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("GRAPI_TEST_S3_ENDPOINT not set")
	}
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	cfg := "upload:\n  config:\n    provider: aws-s3\n    providerOptions:\n      rootPath: media\n      s3Options:\n        endpoint: " + endpoint + "\n        forcePathStyle: true\n        region: us-east-1\n        credentials:\n          accessKeyId: ${env:GRAPI_TEST_S3_KEY}\n          secretAccessKey: ${env:GRAPI_TEST_S3_SECRET}\n        params:\n          Bucket: grapi\n          ACL: private\n"
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(cfg), 0o600); err != nil {
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
	for _, a := range []string{"upload", "destroy"} {
		_, _ = app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api." + a, "role": roleID}, query.Params{})
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	b, ct := pngBody(t, 600, 400, "")
	res, err := http.Post(ts.URL+"/api/upload", ct, b)
	if err != nil {
		t.Fatal(err)
	}
	var files []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&files)
	_ = res.Body.Close()
	if res.StatusCode != 201 || len(files) != 1 {
		t.Fatalf("upload %d %v", res.StatusCode, files)
	}
	f := files[0]
	if f["provider"] != "aws-s3" || f["isUrlSigned"] != true || !strings.Contains(f["url"].(string), "X-Amz-Signature=") {
		t.Fatalf("file %v", f)
	}
	got, err := http.Get(f["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if got.StatusCode != 200 || !bytes.HasPrefix(body, []byte("\x89PNG")) {
		t.Fatalf("signed GET %d (%d bytes)", got.StatusCode, len(body))
	}
	thumb := f["formats"].(map[string]any)["thumbnail"].(map[string]any)
	if got, err := http.Get(thumb["url"].(string)); err != nil || got.StatusCode != 200 {
		t.Fatalf("thumbnail signed GET: %v", err)
	}
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/upload/files/"+jsonNum(f["id"].(float64)), nil)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 200 {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := http.Get(f["url"].(string)); got.StatusCode == 200 {
		t.Fatal("the object is still there after the delete")
	}
}

// s3App sets up the basic fixture with the aws-s3 provider on the test
// store; extra is appended to providerOptions.
func s3App(t *testing.T, endpoint, acl, extra, params string) (*httptest.Server, func()) {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	cfg := "upload:\n  config:\n    provider: aws-s3\n    providerOptions:\n" + extra + "      rootPath: media\n      s3Options:\n        endpoint: " + endpoint + "\n        forcePathStyle: true\n        region: us-east-1\n        credentials:\n          accessKeyId: ${env:GRAPI_TEST_S3_KEY}\n          secretAccessKey: ${env:GRAPI_TEST_S3_SECRET}\n        params:\n          Bucket: grapi\n          ACL: " + acl + "\n" + params
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(cfg), 0o600); err != nil {
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
	ctx := context.Background()
	public, _ := app.Query("plugin::users-permissions.role").FindOne(ctx, query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	for _, a := range []string{"upload", "destroy", "findOne"} {
		_, _ = app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api." + a, "role": roleID}, query.Params{})
	}
	ts := httptest.NewServer(srv.Handler)
	return ts, func() { ts.Close(); _ = srv.Close() }
}

func postUpload(t *testing.T, url, ct string, body io.Reader) map[string]any {
	t.Helper()
	res, err := http.Post(url, ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var files []map[string]any
	var one map[string]any
	if json.Unmarshal(raw, &files) == nil && len(files) == 1 {
		one = files[0]
	} else if json.Unmarshal(raw, &one) != nil {
		t.Fatalf("upload: %d %s", res.StatusCode, raw)
	}
	if res.StatusCode/100 != 2 {
		t.Fatalf("upload: %d %s", res.StatusCode, raw)
	}
	return one
}

func pngWidth(t *testing.T, url string) (int, int) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return res.StatusCode, 0
	}
	cfg, err := png.DecodeConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, cfg.Width
}

func maxRSS() int64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	if runtime.GOOS == "linux" {
		return int64(ru.Maxrss) * 1024
	}
	return int64(ru.Maxrss)
}

func TestS3ReplaceExpiryAndLargeUpload(t *testing.T) {
	endpoint := os.Getenv("GRAPI_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("GRAPI_TEST_S3_ENDPOINT not set")
	}
	ts, stop := s3App(t, endpoint, "private", "", "          signedUrlExpires: 2\n")
	defer stop()

	b, ct := pngBody(t, 600, 400, "")
	f := postUpload(t, ts.URL+"/api/upload", ct, b)
	id := jsonNum(f["id"].(float64))
	b, ct = pngBody(t, 300, 200, "")
	g := postUpload(t, ts.URL+"/api/upload?id="+id, ct, b)
	if g["hash"] != f["hash"] || g["width"] != float64(300) {
		t.Fatalf("replace keeps the hash, with the new image: %v", g)
	}
	if s, w := pngWidth(t, g["url"].(string)); s != 200 || w != 300 {
		t.Fatalf("the store serves the replacement: %d %d", s, w)
	}
	time.Sleep(3 * time.Second)
	if s, _ := pngWidth(t, g["url"].(string)); s == 200 {
		t.Fatal("a signed URL past signedUrlExpires still answers")
	}
	res, err := http.Get(ts.URL + "/api/upload/files/" + id)
	if err != nil {
		t.Fatal(err)
	}
	var again map[string]any
	_ = json.NewDecoder(res.Body).Decode(&again)
	_ = res.Body.Close()
	if s, _ := pngWidth(t, again["url"].(string)); s != 200 {
		t.Fatalf("a new read signs a new URL: %d", s)
	}

	// 200 MB streamed from the client: grapi writes it to a temporary
	// file and streams it to the store.
	const size = 200 * 1000 * 1000
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, _ := mw.CreateFormFile("files", "big.bin")
		chunk := make([]byte, 1<<20)
		_, _ = rand.Read(chunk)
		for written := 0; written < size; written += len(chunk) {
			if _, err := part.Write(chunk[:min(len(chunk), size-written)]); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
		_ = pw.CloseWithError(mw.Close())
	}()
	before := maxRSS()
	big := postUpload(t, ts.URL+"/api/upload", mw.FormDataContentType(), pr)
	grown := maxRSS() - before
	if big["size"] != float64(200000) {
		t.Fatalf("size in KB: %v", big["size"])
	}
	if grown > 100<<20 {
		t.Fatalf("a 200 MB upload grows the peak RSS by %d MB", grown>>20)
	}
	t.Logf("200 MB upload: peak RSS grew by %d MB", grown>>20)
	// It went in parts: the store gives back the whole object.
	res, err = http.Get(big["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || n != size {
		t.Fatalf("read back: %d, %d bytes", res.StatusCode, n)
	}
}

func TestS3BaseURL(t *testing.T) {
	endpoint := os.Getenv("GRAPI_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("GRAPI_TEST_S3_ENDPOINT not set")
	}
	ts, stop := s3App(t, endpoint, "public-read", "      baseUrl: https://cdn.example.test/assets\n", "")
	defer stop()
	b, ct := pngBody(t, 600, 400, "")
	f := postUpload(t, ts.URL+"/api/upload", ct, b)
	if f["isUrlSigned"] == true || !strings.HasPrefix(f["url"].(string), "https://cdn.example.test/assets/media/") {
		t.Fatalf("public file on the CDN: %v", f["url"])
	}
	for name, v := range f["formats"].(map[string]any) {
		if u := v.(map[string]any)["url"].(string); !strings.HasPrefix(u, "https://cdn.example.test/assets/media/") {
			t.Fatalf("format %s not on the CDN: %s", name, u)
		}
	}
}
