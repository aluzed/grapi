package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/typegen"
)

// tsGenerateTypes is `grapi ts:generate-types [-o dir]`: Strapi's
// types/generated/{contentTypes,components}.d.ts.
func tsGenerateTypes(args []string, stdout io.Writer) error {
	fs, dir, _ := projectFlags("ts:generate-types")
	out := fs.String("o", "", "output directory (default <dir>/types/generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, _, err := builtin.Load(os.DirFS(*dir))
	if err != nil {
		return err
	}
	target := *out
	if target == "" {
		target = filepath.Join(*dir, "types", "generated")
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	files := typegen.Generate(res.Set)
	for name, content := range map[string]string{"contentTypes.d.ts": files.ContentTypes, "components.d.ts": files.Components} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "Generated %s and %s in %s\n", "contentTypes.d.ts", "components.d.ts", target)
	return nil
}
