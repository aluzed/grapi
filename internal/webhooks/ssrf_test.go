package webhooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicOnlyTransport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer ts.Close()
	if res, err := http.Get(ts.URL); err != nil || res.StatusCode != 204 {
		t.Fatalf("the default client reaches the loopback: %v", err)
	}
	c := &http.Client{Transport: publicOnlyTransport()}
	_, err := c.Get(ts.URL)
	if err == nil || !strings.Contains(err.Error(), "non public address 127.0.0.1 refused") {
		t.Fatalf("loopback refused: %v", err)
	}
	_, err = c.Get("http://localhost:" + strings.Split(ts.URL, ":")[2])
	if err == nil {
		t.Fatal("a name resolving to the loopback is refused too")
	}
}
