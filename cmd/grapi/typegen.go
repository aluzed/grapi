package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/typegen"
)

// tsGenerateTypes is `grapi ts:generate-types [-o dir]`: Strapi's
// types/generated/{contentTypes,components}.d.ts, and api.d.ts for clients
// that only read the Content API.
func tsGenerateTypes(args []string, stdout io.Writer) error {
	fs, dir, _ := projectFlags("ts:generate-types")
	out := fs.String("o", "", "output directory (default <dir>/types/generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := *out
	if target == "" {
		target = filepath.Join(*dir, "types", "generated")
	}
	if err := writeTypes(*dir, target); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Generated contentTypes.d.ts, components.d.ts and api.d.ts in %s\n", target)
	return nil
}

func writeTypes(dir, target string) error {
	res, _, err := builtin.Load(os.DirFS(dir))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	files := typegen.Generate(res.Set)
	for name, content := range map[string]string{"contentTypes.d.ts": files.ContentTypes, "components.d.ts": files.Components, "api.d.ts": typegen.Client(res.Set)} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// watchTypes writes the types at start, then each time src/ or config/
// changes, as long as the schemas load.
func watchTypes(dir, target string, stderr io.Writer) {
	last := ""
	for {
		if now := grapi.Snapshot(dir); now != last {
			last = now
			if err := writeTypes(dir, target); err != nil {
				fmt.Fprintln(stderr, "grapi develop: types not generated:", err)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
}
