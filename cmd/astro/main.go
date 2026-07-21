// Command astro is the v2 composition root. It lives beside the v1 entry
// point (main.go at the repo root) so the v2 tree builds and tests without
// touching v1; the release still ships one binary — final wiring (after the
// engine lands) registers local.AddCmds on the v1 root and this file goes
// away. Everything the commands need is built here, once, into a Deps
// struct; commands return errors and only this file exits.
package main

import (
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/cmd/local"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	root := local.NewRootCmd(local.NewDeps())
	return root.Execute()
}
