// Package schemasync brings a database in line with the schema grapi derives
// from its models, the way Strapi 5 does at startup: a three-way diff between
// the database, the schema stored by the previous run and the new one.
package schemasync

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/aluzed/grapi/internal/db/metadata"
)

// Hash is the hash Strapi stores next to the schema: SHA-256 of the schema's
// JSON with tables sorted by name in localeCompare order.
func Hash(s *metadata.Schema) (string, error) {
	sorted := &metadata.Schema{Tables: append([]*metadata.Table(nil), s.Tables...)}
	sort.SliceStable(sorted.Tables, func(i, j int) bool {
		return collate(sorted.Tables[i].Name, sorted.Tables[j].Name) < 0
	})
	b, err := metadata.Marshal(sorted)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// collate compares like JavaScript's localeCompare (ICU root collation) on
// identifiers: "_" sorts before digits, digits before letters, letters ignore
// case first and lowercase wins a tie.
func collate(a, b string) int {
	if c := compareWeights(a, b, primary); c != 0 {
		return c
	}
	return compareWeights(a, b, tertiary)
}

func compareWeights(a, b string, weight func(byte) int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if wa, wb := weight(a[i]), weight(b[i]); wa != wb {
			return wa - wb
		}
	}
	return len(a) - len(b)
}

func primary(c byte) int {
	switch {
	case c == '_':
		return 1
	case c >= '0' && c <= '9':
		return 10 + int(c-'0')
	case c >= 'a' && c <= 'z':
		return 100 + int(c-'a')
	case c >= 'A' && c <= 'Z':
		return 100 + int(c-'A')
	}
	return 1000 + int(c)
}

func tertiary(c byte) int {
	if c >= 'A' && c <= 'Z' {
		return 1
	}
	return 0
}
