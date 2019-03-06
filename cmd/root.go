package cmd

import (
	"fmt"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
	"os"
)

var (
	workspaceId string
	rootCmd     = &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
	}
)

func init() {
	cobra.OnInitialize(config.InitConfig)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}