package cmd

import (
	"github.com/astronomerio/astro-cli/config"
	"github.com/spf13/cobra"
)

// RootCmd is the astro root command.
var (
	// Debug       bool
	workspaceId string
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
