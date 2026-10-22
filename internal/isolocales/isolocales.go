// Package isolocales is the i18n plugin's list of ISO locales
// (@strapi/i18n server/src/constants/iso-locales.json, MIT).
package isolocales

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed iso-locales.json
var raw []byte

// Locale is one entry of the list.
type Locale struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var load = sync.OnceValue(func() map[string]string {
	var list []Locale
	_ = json.Unmarshal(raw, &list)
	m := make(map[string]string, len(list))
	for _, l := range list {
		m[l.Code] = l.Name
	}
	return m
})

// Name returns the name of code and whether code is in the list.
func Name(code string) (string, bool) {
	n, ok := load()[code]
	return n, ok
}
