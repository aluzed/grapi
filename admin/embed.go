//go:build adminui

package admin

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func init() {
	FS, _ = fs.Sub(dist, "dist")
}
