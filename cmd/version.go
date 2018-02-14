package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version    = ""
	gitcommit  = ""
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Astronomer CLI version",
		Long:  "Astronomer CLI version",
		RunE:  printVersion,
	}
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

func printVersion(cmd *cobra.Command, args []string) error {
	fmt.Printf("Astro CLI Version: %s\n", version)
	fmt.Printf("Git Commit: %s\n", gitcommit)
	return nil
}
