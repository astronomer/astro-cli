package main

import (
	"os"

	"github.com/astronomerio/astro-cli/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
