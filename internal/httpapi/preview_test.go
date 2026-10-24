package httpapi

import (
	"testing"
	"time"
)

func TestPreviewTokenExpires(t *testing.T) {
	a := &authenticator{previewSecret: "s"}
	tok, err := SignPreviewToken("s", "api::post.post", "abc", time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := a.previewToken(tok); !ok || err != errUnauthorized {
		t.Fatalf("expired: %v %v", ok, err)
	}
	tok, _ = SignPreviewToken("other", "api::post.post", "abc", time.Now())
	if _, ok, err := a.previewToken(tok); !ok || err != errUnauthorized {
		t.Fatalf("forged: %v %v", ok, err)
	}
	tok, _ = SignPreviewToken("s", "api::post.post", "abc", time.Now())
	if g, ok, err := a.previewToken(tok); !ok || err != nil || g.documentID != "abc" {
		t.Fatalf("valid: %v %v %v", g, ok, err)
	}
	if _, ok, _ := a.previewToken("not.a.jwt"); ok {
		t.Fatal("another token is not a preview token")
	}
}
