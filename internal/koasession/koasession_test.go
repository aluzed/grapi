package koasession_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aluzed/grapi/internal/koasession"
)

// Vectors from koa-session's format and keygrip 1.1.0 (node), keys
// ["keyA", "keyB"].
const (
	nodeValue = "eyJkb2N1bWVudGF0aW9uIjp7ImxvZ2dlZCI6dHJ1ZX0sIl9leHBpcmUiOjE3OTI4MDAwMDAwMDAsIl9tYXhBZ2UiOjg2NDAwMDAwfQ=="
	nodeSigA  = "0W_fOq_MMGdacVLRv9IlfkyaZZw"
	nodeSigB  = "410L1Sh2ihtcn2ibflutcTkG620"
)

func TestKeygripSignature(t *testing.T) {
	if got := koasession.Sign("koa.sess="+nodeValue, "keyA"); got != nodeSigA {
		t.Fatalf("keyA: %s, keygrip %s", got, nodeSigA)
	}
	if got := koasession.Sign("koa.sess="+nodeValue, "keyB"); got != nodeSigB {
		t.Fatalf("keyB: %s, keygrip %s", got, nodeSigB)
	}
}

func TestDecodeStrapiCookie(t *testing.T) {
	before := time.UnixMilli(1792800000000 - 1000)
	keys := []string{"keyA", "keyB"}
	for _, sig := range []string{nodeSigA, nodeSigB} {
		data, ok := koasession.Decode(nodeValue, sig, keys, before)
		doc, _ := data["documentation"].(map[string]any)
		if !ok || doc["logged"] != true || data["_expire"] != nil {
			t.Fatalf("sig %s: %v %v", sig, data, ok)
		}
	}
	if _, ok := koasession.Decode(nodeValue, nodeSigA, []string{"other"}, before); ok {
		t.Fatal("a signature from unknown keys is refused")
	}
	if _, ok := koasession.Decode(nodeValue, nodeSigA, keys, time.UnixMilli(1792800000000+1)); ok {
		t.Fatal("an expired session is refused")
	}
}

func TestRoundTrip(t *testing.T) {
	keys := []string{"k1", "k2"}
	rec := httptest.NewRecorder()
	if err := koasession.Write(rec, map[string]any{"documentation": map[string]any{"logged": true}}, 24*time.Hour, keys, false); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		if !c.HttpOnly {
			t.Fatalf("%s is httpOnly", c.Name)
		}
		req.AddCookie(c)
	}
	doc, _ := koasession.Read(req, keys)["documentation"].(map[string]any)
	if doc["logged"] != true {
		t.Fatalf("read back: %v", koasession.Read(req, keys))
	}
	if koasession.Read(httptest.NewRequest("GET", "/", nil), keys) != nil {
		t.Fatal("no cookie, no session")
	}
}
