// Command grapi runs the grapi server and its admin CLI.
package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: grapi <command>\n\ncommands:\n  start               serve the project in the current directory\n  db:sync --dry-run   print what the schema sync would change\n  db:check            check that the database is consistent\n  version             print the grapi version")
		return 2
	}
	switch args[0] {
	case "start":
		if err := start(args[1:], stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "db:sync", "db:check":
		cmd := map[string]func([]string, io.Writer, io.Writer) error{"db:sync": dbSync, "db:check": dbCheck}[args[0]]
		if err := cmd(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	default:
		fmt.Fprintf(stderr, "grapi: unknown command %q\n", args[0])
		return 2
	}
}
