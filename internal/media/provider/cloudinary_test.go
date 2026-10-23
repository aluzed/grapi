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

// A replacement uploads again under the same public_id; a file Strapi
// uploaded is deleted by the public_id and resource_type of its
// provider_metadata, not by its hash.
func TestCloudinaryReplaceAndStrapiMetadata(t *testing.T) {
	var paths []string
	var fields []map[string]string
	version := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		f := map[string]string{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
			for k, v := range r.MultipartForm.Value {
				f[k] = v[0]
			}
		} else {
			_ = r.ParseForm()
			for k, v := range r.PostForm {
				f[k] = v[0]
			}
		}
		fields = append(fields, f)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/destroy") {
			_, _ = io.WriteString(w, `{"result":"ok"}`)
			return
		}
		version++
		_, _ = io.WriteString(w, `{"secure_url":"https://res.cloudinary.com/demo/image/upload/v`+string(rune('0'+version))+`/photo_abc.png","public_id":"photo_abc","resource_type":"image"}`)
	}))
	defer ts.Close()
	c := &Cloudinary{CloudName: "demo", APIKey: "key", APISecret: "secret", BaseURL: ts.URL, now: func() time.Time { return time.Unix(1700000000, 0) }}
	sign := func(s string) string {
		sum := sha1.Sum([]byte(s + "secret")) //nolint:gosec // Cloudinary's signature
		return hex.EncodeToString(sum[:])
	}
	f := &File{Hash: "photo_abc", Ext: ".png"}
	for i := range 2 {
		if err := c.Upload(context.Background(), f, strings.NewReader("png bytes")); err != nil {
			t.Fatal(err)
		}
		if fields[i]["public_id"] != "photo_abc" || fields[i]["signature"] != sign("public_id=photo_abc&timestamp=1700000000") {
			t.Fatalf("upload %d: %v", i, fields[i])
		}
	}
	if f.URL != "https://res.cloudinary.com/demo/image/upload/v2/photo_abc.png" {
		t.Fatalf("the replacement's URL: %s", f.URL)
	}
	strapiFile := &File{Hash: "photo_abc_123", Ext: ".png", Metadata: map[string]any{"public_id": "strapi-uploads/photo_abc_123", "resource_type": "image"}}
	if err := c.Delete(context.Background(), strapiFile); err != nil {
		t.Fatal(err)
	}
	last := fields[len(fields)-1]
	if paths[len(paths)-1] != "/v1_1/demo/image/destroy" || last["public_id"] != "strapi-uploads/photo_abc_123" ||
		last["signature"] != sign("invalidate=true&public_id=strapi-uploads/photo_abc_123&timestamp=1700000000") {
		t.Fatalf("delete of a Strapi file: %s %v", paths[len(paths)-1], last)
	}
}
