// Package koasession reads and writes the session cookie of koa-session,
// as Strapi sets it (strapi::session): the session as base64 JSON in
// koa.sess, signed by keygrip with app.keys in koa.sess.sig.
package koasession

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // keygrip's default algorithm, needed to read Strapi's cookies
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Name is koa-session's default cookie name.
const Name = "koa.sess"

// Sign is keygrip's sign: HMAC-SHA1 of data, base64 URL-safe without
// padding.
func Sign(data, key string) string {
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Encode is the cookie value of a session that expires after maxAge, and
// its signature with the first key.
func Encode(data map[string]any, maxAge time.Duration, keys []string, now time.Time) (value, sig string, err error) {
	if len(keys) == 0 {
		return "", "", errors.New("app.keys is required to sign the session")
	}
	body := map[string]any{}
	for k, v := range data {
		body[k] = v
	}
	body["_expire"] = now.Add(maxAge).UnixMilli()
	body["_maxAge"] = maxAge.Milliseconds()
	b, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	value = base64.StdEncoding.EncodeToString(b)
	return value, Sign(Name+"="+value, keys[0]), nil
}

// Decode checks the signature against every key, as keygrip's index, and
// returns the session without its _expire and _maxAge; false when the
// cookie is forged, malformed or expired.
func Decode(value, sig string, keys []string, now time.Time) (map[string]any, bool) {
	signed := false
	for _, k := range keys {
		if subtle.ConstantTimeCompare([]byte(Sign(Name+"="+value, k)), []byte(sig)) == 1 {
			signed = true
			break
		}
	}
	if !signed {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, false
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return nil, false
	}
	if exp, ok := data["_expire"].(float64); ok && int64(exp) < now.UnixMilli() {
		return nil, false
	}
	delete(data, "_expire")
	delete(data, "_maxAge")
	return data, true
}

// Read is the session of r, nil when there is none or it is not valid.
func Read(r *http.Request, keys []string) map[string]any {
	c, err := r.Cookie(Name)
	if err != nil {
		return nil
	}
	s, err := r.Cookie(Name + ".sig")
	if err != nil {
		return nil
	}
	data, ok := Decode(c.Value, s.Value, keys, time.Now())
	if !ok {
		return nil
	}
	return data
}

// Write sets the session cookies on w, httpOnly, for maxAge (one day by
// default in Strapi's config).
func Write(w http.ResponseWriter, data map[string]any, maxAge time.Duration, keys []string, secure bool) error {
	value, sig, err := Encode(data, maxAge, keys, time.Now())
	if err != nil {
		return err
	}
	expires := time.Now().Add(maxAge)
	for name, v := range map[string]string{Name: value, Name + ".sig": sig} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: v, Path: "/", Expires: expires, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode}) //nolint:gosec // Secure follows the scheme, as koa-session does
	}
	return nil
}
