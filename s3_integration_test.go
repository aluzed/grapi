//go:build integration

package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/query"
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
	e := app.Documents().E
	public, _ := e.FindOne(ctx, "plugin::users-permissions.role", query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	for _, a := range []string{"upload", "destroy"} {
		_, _ = e.Create(ctx, "plugin::users-permissions.permission", query.Data{"action": "plugin::upload.content-api." + a, "role": roleID}, query.Params{})
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
