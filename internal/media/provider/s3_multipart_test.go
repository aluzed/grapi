package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeS3 struct {
	mu        sync.Mutex
	parts     map[string]int64
	completed string
	aborted   bool
	failPart  string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodPost && q.Has("uploads"):
		_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>up-1</UploadId></InitiateMultipartUploadResult>`)
	case r.Method == http.MethodPut && q.Get("uploadId") == "up-1":
		n, _ := io.Copy(io.Discard, r.Body)
		if q.Get("partNumber") == f.failPart {
			w.WriteHeader(500)
			return
		}
		f.parts[q.Get("partNumber")] = n
		w.Header().Set("ETag", `"etag-`+q.Get("partNumber")+`"`)
	case r.Method == http.MethodPost && q.Get("uploadId") == "up-1":
		b, _ := io.ReadAll(r.Body)
		f.completed = string(b)
		_, _ = io.WriteString(w, `<CompleteMultipartUploadResult></CompleteMultipartUploadResult>`)
	case r.Method == http.MethodDelete && q.Get("uploadId") == "up-1":
		f.aborted = true
		w.WriteHeader(204)
	default:
		w.WriteHeader(400)
	}
}

func TestS3Multipart(t *testing.T) {
	fake := &fakeS3{parts: map[string]int64{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	p := &S3{Bucket: "b", Region: "us-east-1", AccessKey: "k", Secret: "s", Endpoint: srv.URL, ForcePathStyle: true}
	size := int64(multipartThreshold + multipartPartSize + 123)
	f := &File{Hash: "big", Ext: ".bin", Mime: "application/octet-stream", Size: size}
	if err := p.Upload(context.Background(), f, io.LimitReader(zeros{}, size)); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, n := range fake.parts {
		total += n
	}
	if len(fake.parts) != 6 || total != size || fake.parts["6"] != 123 {
		t.Fatalf("parts: %v", fake.parts)
	}
	if !strings.Contains(fake.completed, "<PartNumber>6</PartNumber>") || !strings.Contains(fake.completed, `&#34;etag-1&#34;`) || fake.aborted {
		t.Fatalf("complete: %s aborted=%v", fake.completed, fake.aborted)
	}
	if !strings.HasSuffix(f.URL, "/b/big.bin") {
		t.Fatalf("url: %s", f.URL)
	}

	failing := &fakeS3{parts: map[string]int64{}, failPart: "2"}
	srv2 := httptest.NewServer(failing)
	defer srv2.Close()
	p.Endpoint = srv2.URL
	if err := p.Upload(context.Background(), &File{Hash: "big", Ext: ".bin", Size: size}, io.LimitReader(zeros{}, size)); err == nil || !failing.aborted || failing.completed != "" {
		t.Fatalf("a failed part aborts: %v aborted=%v", err, failing.aborted)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
