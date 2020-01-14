package cmd

import (
	"io"

	"github.com/sjmiller609/astro-cli/houston"
	"github.com/spf13/cobra"
)

func newSaRootCmd(_ *houston.Client, _ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Deprecated: `please use commands instead: 
  $ astro workspace service-account 
  or 
  $ astro deployment service-account
`,
	}
	return cmd
}
