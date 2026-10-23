package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/aluzed/grapi"
)

// memoryProvider keeps the files in memory, under a fake CDN.
type memoryProvider struct {
	mu      sync.Mutex
	files   map[string][]byte
	deleted []string
}

func (p *memoryProvider) Upload(_ context.Context, f *grapi.UploadFile, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.files[f.Hash+f.Ext] = b
	f.URL = "https://cdn.example.com/" + f.Hash + f.Ext
	return nil
}

func (p *memoryProvider) Delete(_ context.Context, f *grapi.UploadFile) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.files, f.Hash+f.Ext)
	p.deleted = append(p.deleted, f.Hash+f.Ext)
	return nil
}

func (p *memoryProvider) IsPrivate() bool { return false }

func (p *memoryProvider) SignedURL(_ context.Context, f *grapi.UploadFile) (string, error) {
	return f.URL, nil
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{B: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCustomUploadProvider(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("upload:\n  config:\n    provider: memory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	t.Setenv("ADMIN_JWT_SECRET", "admin-test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	mem := &memoryProvider{files: map[string][]byte{}}
	app.UseUploadProvider("memory", mem)
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer func() { ts.Close(); _ = srv.Close() }()

	res, err := http.Post(ts.URL+"/admin/api/v1/auth/register-admin", "application/json", strings.NewReader(`{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct{ Data struct{ Token string } }
	_ = json.NewDecoder(res.Body).Decode(&reg)
	_ = res.Body.Close()
	upload := func(path string, content []byte) map[string]any {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("files", "pic.png")
		_, _ = fw.Write(content)
		_ = mw.Close()
		req, _ := http.NewRequest("POST", ts.URL+"/admin/api/v1"+path, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+reg.Data.Token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		var list []map[string]any
		if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 {
			var one map[string]any
			if json.Unmarshal(raw, &one) == nil && one["id"] != nil {
				return one
			}
			t.Fatalf("upload %s: %d %s", path, res.StatusCode, raw)
		}
		return list[0]
	}
	f := upload("/upload", pngOf(t, 1200, 800))
	hash := f["hash"].(string)
	if url, _ := f["url"].(string); url != "https://cdn.example.com/"+hash+".png" {
		t.Fatalf("the custom provider stores the file: %v", f["url"])
	}
	stored := func() []string {
		mem.mu.Lock()
		defer mem.mu.Unlock()
		var out []string
		for k := range mem.files {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	if n := len(stored()); n < 3 {
		t.Fatalf("the original and its formats: %v", stored())
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "uploads", hash+".png")); err == nil {
		t.Fatal("nothing goes to the local folder")
	}
	g := upload("/upload?id="+jsonNumber(f["id"]), pngOf(t, 40, 30))
	if g["hash"] != hash {
		t.Fatalf("a replaced file keeps its hash: %v vs %v", g["hash"], hash)
	}
	if formats, _ := g["formats"].(map[string]any); len(formats) != 0 {
		t.Fatalf("the formats follow the new content: %v", formats)
	}
	if got := stored(); len(got) != 1 || got[0] != hash+".png" {
		t.Fatalf("the old formats are removed from the provider: %v", got)
	}
}

func jsonNumber(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
