package webhooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicOnlyAddresses(t *testing.T) {
	for address, allowed := range map[string]bool{
		"127.0.0.1:80": false, "10.0.0.1:80": false, "169.254.169.254:80": false,
		"0.1.2.3:80": false, "0.0.0.0:80": false, "[::ffff:0.1.2.3]:80": false,
		"100.64.0.1:80": false, "100.127.255.255:80": false, "[::ffff:100.64.0.1]:80": false,
		"[::1]:80": false, "[fd00::1]:80": false, "[fe80::1]:80": false,
		"224.0.0.1:80": false, "localhost:80": false, "invalid": false,
		"93.184.216.34:443": true, "[2606:4700::1111]:443": true,
		"100.63.255.255:80": true, "100.128.0.0:80": true,
	} {
		if err := publicOnly("tcp", address, nil); (err == nil) != allowed {
			t.Errorf("%s: %v", address, err)
		}
	}
}

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
