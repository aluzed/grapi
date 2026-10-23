package httpapi

import (
	"testing"
)

func TestPrefixKey(t *testing.T) {
	cases := map[string]string{
		"/api/auth/local":               "noIdentifier:/api/auth/local:1.2.3.4",
		"/api/auth/local/":              "noIdentifier:/api/auth/local:1.2.3.4",
		"/api/Auth/Change-Password":     "noIdentifier:/api/auth/change-password:1.2.3.4",
		"/api/auth/forgot-password":     "a@b.c:/api/auth/forgot-password:1.2.3.4",
		"/api/auth/local/register":      "a@b.c:/api/auth/local/register:1.2.3.4",
		"/api/connect/github/callback/": "noIdentifier:/api/connect/github/callback:1.2.3.4",
	}
	for p, want := range cases {
		if got := prefixKey(p, "1.2.3.4", []byte(`{"email":"A@b.c"}`)); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
	if got := prefixKey("/api/auth/forgot-password", "ip", []byte(`{}`)); got != "unknownIdentifier:/api/auth/forgot-password:ip" {
		t.Errorf("no email: %s", got)
	}
}
