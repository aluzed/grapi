package main

import (
	"io"
	"os"

	"github.com/aluzed/grapi/internal/project"
)

// openAPI is `grapi openapi [-o file]`: the content API's document, on
// stdout by default.
func openAPI(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("openapi")
	out := fs.String("o", "", "output file (stdout by default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := project.Load(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	doc, err := p.OpenAPI()
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = stdout.Write(append(doc, '\n'))
		return err
	}
	return os.WriteFile(*out, append(doc, '\n'), 0o600)
}
