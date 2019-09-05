package cmd

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

// RootCmd is the astro root command.
var (
	workspaceId   string
	workspaceRole string
	role          string
	RootCmd     = &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
	}
)

func init() {
	cobra.OnInitialize(config.InitConfig)
	// RootCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "debug output")
}
