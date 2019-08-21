package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generates bash completion scripts",
	Long: `To load completion run

. <(astro completion)

To configure your bash shell to load completions for each session add to your bashrc

# ~/.bashrc or ~/.profile
. <(astro completion)`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = RootCmd.GenBashCompletion(os.Stdout)
	},
}

func init() {
	RootCmd.AddCommand(completionCmd)
}
