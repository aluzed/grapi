package httpapi

import (
	"testing"
	"time"
)

func TestTokenExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	token := expiringToken(20, time.Hour, now)
	for at, valid := range map[time.Duration]bool{0: true, 59 * time.Minute: true, time.Hour: false, 2 * time.Hour: false} {
		if got := tokenValid(token, time.Hour, now.Add(at)); got != valid {
			t.Errorf("after %s: %v", at, got)
		}
	}
	legacy := expiringToken(20, 0, now)
	if !tokenValid(legacy, 0, now.Add(1000*time.Hour)) {
		t.Error("without a lifetime, tokens never expire")
	}
	if tokenValid(legacy, time.Hour, now) {
		t.Error("with a lifetime, a token without expiry is refused")
	}
	for _, bad := range []any{"ab.", "ab.x", "ab.1.2", nil, 3} {
		if tokenValid(bad, time.Hour, now) {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestPasswordStamp(t *testing.T) {
	a := passwordStamp("secret", "$2a$10$hash1")
	if a != passwordStamp("secret", "$2a$10$hash1") || a == passwordStamp("secret", "$2a$10$hash2") || a == passwordStamp("other", "$2a$10$hash1") {
		t.Fatal("the stamp depends on the hash and the secret only")
	}
}
