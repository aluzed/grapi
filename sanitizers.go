package grapi

import (
	"fmt"

	"github.com/aluzed/grapi/internal/httpapi"
)

// Sanitizer is a function of strapi.sanitizers.add: it gets the model uid
// and one entry, after the built-in rules, and returns the entry to keep.
type Sanitizer = httpapi.Sanitizer

// AddSanitizer is strapi.sanitizers.add: kind is "content-api.input" (the
// data of a write) or "content-api.output" (each top-level entry of a
// response), as in Strapi. They run in registration order.
func (a *App) AddSanitizer(kind string, fn Sanitizer) {
	switch kind {
	case "content-api.input":
		a.sanitizers.Input = append(a.sanitizers.Input, fn)
	case "content-api.output":
		a.sanitizers.Output = append(a.sanitizers.Output, fn)
	default:
		panic(fmt.Sprintf("grapi: unknown sanitizer kind %q (content-api.input or content-api.output)", kind))
	}
}
