package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/context"
)

var noPrompt bool

func newContextCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"c"},
		Short:   "Manage Astro & Astro Private Cloud contexts",
		Long:    "Context represent a connection to Astro or Astro Private Cloud in the form of a Domain URL. If your context is set to astronomer.io, for example, you are connected to Astro",
	}
	cmd.AddCommand(
		newContextListCmd(out),
		newContextSwitchCmd(),
		newContextDeleteCmd(),
	)
	return cmd
}

func newContextListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all contexts",
		Long:    "List all Astro and Astro Private Cloud contexts or domains that you've authenticated to on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.ListContext(cmd, args, out)
		},
	}
	return cmd
}

func newContextSwitchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [domain]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different context",
		Long:    "Switch to a different context",
		RunE:    context.SwitchContext,
		Args:    cobra.MaximumNArgs(1),
	}
	return cmd
}

func newContextDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [domain]",
		Aliases: []string{"de"},
		Short:   "Delete a context",
		Long:    "Delete a locally stored context to Astro or Astro Private Cloud",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.DeleteContext(cmd, args, noPrompt)
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().BoolVarP(&noPrompt, "force", "f", false, "Don't prompt a user before context delete; assume \"yes\" as answer to all prompts and run non-interactively.")
	return cmd
}
