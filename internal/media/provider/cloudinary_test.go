package provider

import (
	"context"
	"crypto/sha1" //nolint:gosec // Cloudinary's signature
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudinary(t *testing.T) {
	var paths []string
	var fields []map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		f := map[string]string{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
			for k, v := range r.MultipartForm.Value {
				f[k] = v[0]
			}
			file, _, _ := r.FormFile("file")
			b, _ := io.ReadAll(file)
			f["file"] = string(b)
		} else {
			_ = r.ParseForm()
			for k, v := range r.PostForm {
				f[k] = v[0]
			}
		}
		fields = append(fields, f)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/destroy") {
			_, _ = io.WriteString(w, `{"result":"not found"}`)
			return
		}
		_, _ = io.WriteString(w, `{"secure_url":"https://res.cloudinary.com/demo/video/upload/abc.mp4","public_id":"abc","resource_type":"video"}`)
	}))
	defer ts.Close()
	c := &Cloudinary{
		CloudName: "demo", APIKey: "key", APISecret: "secret", BaseURL: ts.URL, UploadOptions: map[string]any{"folder": "strapi"},
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}
	f := &File{Hash: "abc", Ext: ".mp4"}
	if err := c.Upload(context.Background(), f, strings.NewReader("bytes")); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum([]byte("folder=strapi&public_id=abc&timestamp=1700000000secret")) //nolint:gosec // Cloudinary's signature
	up := fields[0]
	if paths[0] != "/v1_1/demo/auto/upload" || up["signature"] != hex.EncodeToString(sum[:]) || up["api_key"] != "key" || up["file"] != "bytes" {
		t.Fatalf("upload %s %v", paths[0], up)
	}
	if f.URL != "https://res.cloudinary.com/demo/video/upload/abc.mp4" || f.Metadata["resource_type"] != "video" || f.Metadata["public_id"] != "abc" {
		t.Fatalf("file %+v", f)
	}
	if err := c.Delete(context.Background(), f); err != nil {
		t.Fatalf("a missing file is not an error: %v", err)
	}
	if paths[1] != "/v1_1/demo/video/destroy" || fields[1]["public_id"] != "abc" || fields[1]["invalidate"] != "true" {
		t.Fatalf("delete %s %v", paths[1], fields[1])
	}
}
