package media

import (
	"crypto/rand"
	"encoding/hex"
	"math"

	"github.com/aluzed/grapi/internal/slug"
)

// GenerateFileName is the upload plugin's generateFileName: the slugified
// base name, case kept, and 10 random hex characters.
func GenerateFileName(name string) string {
	o := slug.Defaults()
	o.Separator, o.Lowercase = "_", false
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return slug.Make(name, o) + "_" + hex.EncodeToString(b)
}

// BytesToKbytes is @strapi/utils' bytesToKbytes: kilobytes, 2 decimals.
func BytesToKbytes(n int64) float64 {
	return math.Round(float64(n)/1000*100) / 100
}
