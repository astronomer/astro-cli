package main

import (
	"os"

	"github.com/astronomer/astro-cli/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
