package adminapi

import (
	"net/mail"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10

// checkPassword is the admin password policy; bcrypt reads 72 bytes at most,
// a longer password would be silently truncated.
func checkPassword(field, p string) []FieldError {
	var errs []FieldError
	add := func(msg string) { errs = append(errs, FieldError{Path: []string{field}, Message: msg}) }
	if len(p) < 8 {
		add(field + " must be at least 8 characters")
	}
	if len([]byte(p)) > 72 {
		add(field + " must be less than 73 bytes")
	}
	var lower, upper, digit bool
	for _, r := range p {
		lower = lower || unicode.IsLower(r)
		upper = upper || unicode.IsUpper(r)
		digit = digit || unicode.IsDigit(r)
	}
	if !lower {
		add(field + " must contain at least one lowercase character")
	}
	if !upper {
		add(field + " must contain at least one uppercase character")
	}
	if !digit {
		add(field + " must contain at least one number")
	}
	return errs
}

func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcryptCost)
	return string(h), err
}

func passwordMatches(p, hash string) bool {
	return hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func checkEmail(field, e string) []FieldError {
	if a, err := mail.ParseAddress(e); err != nil || a.Address != e || !strings.Contains(e, "@") {
		return []FieldError{{Path: []string{field}, Message: field + " must be a valid email"}}
	}
	return nil
}
