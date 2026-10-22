package provider

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The presigned GET example of the AWS SigV4 documentation
// ("Authenticating Requests: Using Query Parameters").
func TestPresignMatchesAWSExample(t *testing.T) {
	p := &S3{
		Bucket: "examplebucket", Region: "us-east-1",
		AccessKey: "AKIAIOSFODNN7EXAMPLE", Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SignedURLExpires: 86400 * time.Second,
		now:              func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	}
	u, err := p.SignedURL(context.Background(), &File{Hash: "test", Ext: ".txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://examplebucket.s3.amazonaws.com/test.txt?") {
		t.Fatalf("url %s", u)
	}
	if !strings.Contains(u, "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404") {
		t.Fatalf("signature differs from the AWS example: %s", u)
	}
}

func TestKeyAndURL(t *testing.T) {
	p := &S3{Bucket: "b", Region: "eu-west-3", RootPath: "media/"}
	f := &File{Hash: "photo_ab12", Ext: ".png", Path: "../x"}
	if k := p.Key(f); k != "media/x/photo_ab12.png" {
		t.Fatalf("key %s", k)
	}
	if u := p.fileURL(p.Key(f)); u != "https://b.s3.eu-west-3.amazonaws.com/media/x/photo_ab12.png" {
		t.Fatalf("url %s", u)
	}
	p.Endpoint, p.ForcePathStyle = "http://127.0.0.1:9000", true
	if u := p.fileURL("k.png"); u != "http://127.0.0.1:9000/b/k.png" {
		t.Fatalf("path style %s", u)
	}
	p.BaseURL = "https://cdn.example.com/"
	if u := p.fileURL("k.png"); u != "https://cdn.example.com/k.png" {
		t.Fatalf("baseUrl %s", u)
	}
}
