// Package naming derives table, column and index names exactly like Strapi 5:
// a different name for the same schema would make schema sync drop data.
package naming

import (
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// MaxLength is Strapi's identifier limit. 0 disables shortening (Strapi 4 names).
const MaxLength = 55

const (
	hashLength     = 5
	minTokenLength = 3
	separator      = "_"
)

var shortNames = map[string]string{
	"links":           "lnk",
	"order_inv_fk":    "oifk",
	"order":           "ord",
	"morphs":          "mph",
	"index":           "idx",
	"inv_fk":          "ifk",
	"order_fk":        "ofk",
	"id_column_index": "idix",
	"order_index":     "oidx",
	"unique":          "uq",
	"primary":         "pk",
}

// Options adds a fixed prefix or suffix: they are never truncated, but the
// known ones ("links", "order", "morphs"...) use their short form when the full
// name does not fit.
type Options struct {
	Prefix string
	Suffix string
}

// Namer builds names for one maximum length.
type Namer struct{ maxLength int }

// New returns a Namer; Default uses Strapi's 55 characters.
func New(maxLength int) Namer { return Namer{maxLength: maxLength} }

// Default is the namer Strapi 5 uses.
var Default = New(MaxLength)

type token struct {
	name         string
	compressible bool
	shortName    string
	allocated    int
}

// Name joins names with "_" and shortens the result to fit.
func (n Namer) Name(names []string, o Options) (string, error) {
	tokens := make([]*token, 0, len(names)+2)
	if o.Prefix != "" {
		tokens = append(tokens, &token{name: o.Prefix, shortName: shortNames[o.Prefix]})
	}
	for _, name := range names {
		tokens = append(tokens, &token{name: name, compressible: true})
	}
	if o.Suffix != "" {
		tokens = append(tokens, &token{name: o.Suffix, shortName: shortNames[o.Suffix]})
	}
	return n.fromTokens(tokens)
}

func join(tokens []*token, part func(*token) string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = part(t)
	}
	return strings.Join(parts, separator)
}

func (t *token) fixed() string {
	if !t.compressible && t.shortName != "" {
		return t.shortName
	}
	return t.name
}

var errTooSmall = errors.New("maximum length is too small to accommodate all tokens")

func (n Namer) fromTokens(tokens []*token) (string, error) {
	if n.maxLength < 0 {
		return "", errors.New("maxLength must be a positive integer or 0 (for unlimited length)")
	}
	if n.maxLength == 0 {
		return join(tokens, func(t *token) string { return t.name }), nil
	}
	full := join(tokens, (*token).fixed)
	if len(full) <= n.maxLength {
		return full, nil
	}

	var compressible []*token
	incompressibleLength := 0
	for _, t := range tokens {
		if t.compressible {
			compressible = append(compressible, t)
		} else {
			incompressibleLength += len(t.fixed())
		}
	}
	separators := len(tokens)*len(separator) - 1
	available := n.maxLength - incompressibleLength - separators
	if len(compressible) == 0 || incompressibleLength+separators > n.maxLength {
		return "", errTooSmall
	}
	perToken := available / len(compressible)
	if perToken < minTokenLength {
		return "", errTooSmall
	}
	surplus := available % len(compressible)

	minHashed := hashLength + minTokenLength
	total := separators
	for _, t := range tokens {
		switch {
		case !t.compressible:
			total += len(t.fixed())
		case len(t.name) < perToken:
			total += len(t.name)
		default:
			total += minHashed
		}
	}
	if n.maxLength < total {
		return "", errTooSmall
	}

	var deficits []*token
	for _, t := range compressible {
		if len(t.name) < perToken {
			surplus += perToken - len(t.name)
			t.allocated = len(t.name)
		} else {
			t.allocated = perToken
			deficits = append(deficits, t)
		}
	}
	// One character per round to each token still short of its full name, in order.
	previous := surplus + 1
	for surplus > 0 && len(deficits) > 0 {
		next := deficits[:0]
		for _, t := range deficits {
			if t.allocated < len(t.name) && surplus > 0 {
				t.allocated++
				surplus--
				if t.allocated < len(t.name) {
					next = append(next, t)
				}
			}
		}
		deficits = next
		if surplus == previous {
			break
		}
		previous = surplus
	}

	var err error
	name := join(tokens, func(t *token) string {
		if t.compressible {
			s, e := shorten(t.name, t.allocated)
			if e != nil {
				err = e
			}
			return s
		}
		return t.fixed()
	})
	if err != nil {
		return "", err
	}
	if len(name) > n.maxLength {
		return "", fmt.Errorf("name shortening failed to generate a name of the correct maxLength; name %s", name)
	}
	return name, nil
}

// shorten keeps the start of name and appends a 5 character hash of the full name.
func shorten(name string, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("tokenWithHash length must be a positive integer, received %d", length)
	}
	if len(name) <= length {
		return name, nil
	}
	if length < minTokenLength+hashLength {
		return "", fmt.Errorf("length for part of identifier too short, minimum is hash length (%d) plus min token length (%d), received %d for token %s", hashLength, minTokenLength, length, name)
	}
	return name[:length-hashLength] + Hash(name, hashLength), nil
}

// Hash is the first length hex characters of SHAKE256(data).
func Hash(data string, length int) string {
	out := make([]byte, (length+1)/2)
	h := sha3.NewSHAKE256()
	_, _ = h.Write([]byte(data))
	_, _ = h.Read(out)
	return hex.EncodeToString(out)[:length]
}

// Token is one part of a name built token by token, as Strapi does for
// component link tables and their columns.
type Token struct {
	Name         string
	ShortName    string
	Compressible bool
}

// FromTokens joins tokens with "_" and shortens the compressible ones to fit.
func (n Namer) FromTokens(tokens ...Token) (string, error) {
	ts := make([]*token, len(tokens))
	for i, t := range tokens {
		ts[i] = &token{name: t.Name, shortName: t.ShortName, compressible: t.Compressible}
	}
	return n.fromTokens(ts)
}
