package main

import (
	"os"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/spf13/afero"
)

func main() {
	// TODO: Remove this when version logic is implemented
	fs := afero.NewOsFs()
	config.InitConfig(fs)
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}

	// platform specific terminal initialization:
	// this should run for all commands,
	// for most of the architectures there's no requirements:
	ansi.InitConsole()
}
