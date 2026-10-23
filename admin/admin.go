// Package admin holds the admin SPA. The binary embeds its build with the
// adminui build tag (make build); without it, FS is nil and /admin serves
// a placeholder page.
package admin

import "io/fs"

// FS is the built SPA, rooted at its index.html.
var FS fs.FS
