// Package users holds the end-user side of users-permissions: JWTs, the
// plugin settings and password rules.
package users

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidToken is every verification failure, as users-permissions
// reports them all as "Invalid token.".
var ErrInvalidToken = errors.New("invalid token")

var msPattern = regexp.MustCompile(`(?i)^(-?(?:\d+)?\.?\d+) *(milliseconds?|msecs?|ms|seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d|weeks?|w|years?|yrs?|y)?$`)

// ParseDuration is the ms package that jsonwebtoken uses for expiresIn.
func ParseDuration(s string) (time.Duration, error) {
	m := msPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || len(s) > 100 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}
	const day = 24 * float64(time.Hour)
	unit := float64(time.Millisecond)
	switch strings.ToLower(m[2]) {
	case "years", "year", "yrs", "yr", "y":
		unit = day * 365.25
	case "weeks", "week", "w":
		unit = day * 7
	case "days", "day", "d":
		unit = day
	case "hours", "hour", "hrs", "hr", "h":
		unit = float64(time.Hour)
	case "minutes", "minute", "mins", "min", "m":
		unit = float64(time.Minute)
	case "seconds", "second", "secs", "sec", "s":
		unit = float64(time.Second)
	}
	return time.Duration(n * unit), nil
}

var algorithms = map[string]func() hash.Hash{"HS256": sha256.New, "HS384": sha512.New384, "HS512": sha512.New}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Sign issues a token like jsonwebtoken.sign(payload, secret, {expiresIn,
// algorithm}): iat then exp are appended to the payload keys.
func Sign(payload []KV, secret, algorithm, expiresIn string, now time.Time) (string, error) {
	if algorithm == "" {
		algorithm = "HS256"
	}
	newHash, ok := algorithms[algorithm]
	if !ok {
		return "", fmt.Errorf("unsupported jwt algorithm %s", algorithm)
	}
	iat := now.Unix()
	fields := append([]KV(nil), payload...)
	fields = append(fields, KV{"iat", iat})
	if expiresIn != "" {
		var seconds float64
		if n, err := strconv.ParseFloat(expiresIn, 64); err == nil {
			seconds = n
		} else {
			d, err := ParseDuration(expiresIn)
			if err != nil {
				return "", err
			}
			seconds = d.Seconds()
		}
		fields = append(fields, KV{"exp", int64(math.Floor(float64(iat) + seconds))})
	}
	header := b64([]byte(`{"alg":"` + algorithm + `","typ":"JWT"}`))
	body, err := marshalOrdered(fields)
	if err != nil {
		return "", err
	}
	signing := header + "." + b64(body)
	mac := hmac.New(newHash, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + b64(mac.Sum(nil)), nil
}

// KV is one payload field; the order is the JSON order.
type KV struct {
	Key   string
	Value any
}

func marshalOrdered(fields []KV) ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.Key)
		v, err := json.Marshal(f.Value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Verify checks a token like jsonwebtoken.verify with one allowed algorithm
// and returns its payload.
func Verify(token, secret, algorithm string, now time.Time) (map[string]any, error) {
	if algorithm == "" {
		algorithm = "HS256"
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || secret == "" {
		return nil, ErrInvalidToken
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(rawHeader, &header) != nil || header.Alg != algorithm {
		return nil, ErrInvalidToken
	}
	newHash, ok := algorithms[algorithm]
	if !ok {
		return nil, ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	mac := hmac.New(newHash, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, ErrInvalidToken
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var payload map[string]any
	if json.Unmarshal(rawPayload, &payload) != nil || payload == nil {
		return nil, ErrInvalidToken
	}
	clock := float64(now.Unix())
	if v, ok := payload["nbf"]; ok {
		n, isNum := v.(float64)
		if !isNum || n > clock {
			return nil, ErrInvalidToken
		}
	}
	if v, ok := payload["exp"]; ok {
		n, isNum := v.(float64)
		if !isNum || clock >= n {
			return nil, ErrInvalidToken
		}
	}
	return payload, nil
}
