package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type captured struct {
	r    *http.Request
	body []byte
}

func server(t *testing.T, got *captured) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.r = r
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(ts.Close)
	return ts
}

var msg = Message{From: "Ada <ada@example.com>", To: "bob@example.com, Carol <carol@example.com>", Cc: "dan@example.com", ReplyTo: "reply@example.com", Subject: "Hi", Text: "plain", HTML: "<p>html</p>"}

func TestSendGrid(t *testing.T) {
	var got captured
	ts := server(t, &got)
	if err := (&SendGrid{APIKey: "SG.key", BaseURL: ts.URL}).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if got.r.URL.Path != "/v3/mail/send" || got.r.Header.Get("Authorization") != "Bearer SG.key" {
		t.Fatalf("%s %s", got.r.URL.Path, got.r.Header.Get("Authorization"))
	}
	var b map[string]any
	_ = json.Unmarshal(got.body, &b)
	want := `{"content":[{"type":"text/plain","value":"plain"},{"type":"text/html","value":"<p>html</p>"}],"from":{"email":"ada@example.com","name":"Ada"},"personalizations":[{"cc":[{"email":"dan@example.com"}],"to":[{"email":"bob@example.com"},{"email":"carol@example.com","name":"Carol"}]}],"reply_to":{"email":"reply@example.com"},"subject":"Hi"}`
	var w map[string]any
	_ = json.Unmarshal([]byte(want), &w)
	exp, _ := json.Marshal(w)
	if out, _ := json.Marshal(b); string(out) != string(exp) {
		t.Fatalf("body %s", out)
	}
}

func TestMailgun(t *testing.T) {
	var got captured
	ts := server(t, &got)
	if err := (&Mailgun{Key: "key-1", URL: ts.URL, Domain: "mg.example.com"}).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	user, pass, _ := got.r.BasicAuth()
	form, _ := url.ParseQuery(string(got.body))
	if got.r.URL.Path != "/v3/mg.example.com/messages" || user != "api" || pass != "key-1" || form.Get("h:Reply-To") != "reply@example.com" || form.Get("to") != msg.To || form.Get("html") != msg.HTML {
		t.Fatalf("%s %s:%s %v", got.r.URL.Path, user, pass, form)
	}
}

func TestSES(t *testing.T) {
	var got captured
	ts := server(t, &got)
	p := &SES{Key: "AKID", Secret: "secret", Endpoint: ts.URL, now: func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }}
	if err := p.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	form, _ := url.ParseQuery(string(got.body))
	if form.Get("Action") != "SendEmail" || form.Get("Destination.ToAddresses.member.2") != `"Carol" <carol@example.com>` || form.Get("Message.Body.Text.Data") != "plain" {
		t.Fatalf("form %v", form)
	}
	auth := got.r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKID/20261022/us-east-1/ses/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=") {
		t.Fatalf("authorization %s", auth)
	}
	if (&SES{}).region("email.eu-west-3.amazonaws.com") != "eu-west-3" {
		t.Fatal("region from the endpoint")
	}
}

func TestProviderError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer ts.Close()
	err := (&SendGrid{APIKey: "x", BaseURL: ts.URL}).Send(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("an API error is returned: %v", err)
	}
}
