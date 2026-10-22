package users

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"30d": 30 * 24 * time.Hour, "7h": 7 * time.Hour, "60s": time.Minute, "1.5h": 90 * time.Minute,
		"100": 100 * time.Millisecond, "2 weeks": 14 * 24 * time.Hour, "1y": time.Duration(365.25 * 24 * float64(time.Hour)),
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDuration("soon"); err == nil {
		t.Error("want an error")
	}
}

// Signed by jsonwebtoken 9.0.3 (Strapi 5.59's), on the homelab:
// jwt.sign({id: 1, iat: 1760000000}, "s", {expiresIn: "30d"}).
const strapiToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6MSwiaWF0IjoxNzYwMDAwMDAwLCJleHAiOjE3NjI1OTIwMDB9.nsaLPyIQ1up1YBkXY_ICYOv6XLmem0XOvAz9EepNeyg"

func TestSignMatchesJsonwebtoken(t *testing.T) {
	tok, err := Sign([]KV{{"id", 1}}, "s", "HS256", "30d", time.Unix(1760000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if tok != strapiToken {
		t.Fatalf("token = %s\nwant %s", tok, strapiToken)
	}
	p, err := Verify(tok, "s", "", time.Unix(1760000100, 0))
	if err != nil || p["id"] != float64(1) {
		t.Fatalf("Verify = %v, %v", p, err)
	}
	for name, check := range map[string]func() error{
		"expired":   func() error { _, err := Verify(tok, "s", "", time.Unix(1762592000, 0)); return err },
		"secret":    func() error { _, err := Verify(tok, "other", "", time.Unix(1760000100, 0)); return err },
		"algorithm": func() error { _, err := Verify(tok, "s", "HS512", time.Unix(1760000100, 0)); return err },
		"garbage":   func() error { _, err := Verify("not.a.jwt", "s", "", time.Now()); return err },
	} {
		if check() == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
