package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/context"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/domainutil"
)

var (
	noPrompt bool

	cloudSwitch = astroAuth.Switch
)

func newContextCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"c"},
		Short:   "Manage Astro & APC contexts",
		Long:    "Context represent a connection to Astro or APC in the form of a Domain URL. If your context is set to astronomer.io, for example, you are connected to Astro",
	}
	cmd.AddCommand(
		newContextListCmd(out),
		newContextSwitchCmd(astroV1Client, out),
		newContextDeleteCmd(),
	)
	return cmd
}

func newContextListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all contexts",
		Long:    "List all Astro and APC contexts or domains that you've authenticated to on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.ListContext(cmd, args, out)
		},
	}
	return cmd
}

func newContextSwitchCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [domain]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different context",
		Long:    "Switch to a different context. For Astro, the saved login for the domain is refreshed if it can be; the command never opens a browser.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchContext(cmd, args, astroV1Client, out)
		},
		Args: cobra.MaximumNArgs(1),
	}
	return cmd
}

func switchContext(cmd *cobra.Command, args []string, astroV1Client astrov1.APIClient, out io.Writer) error {
	if len(args) == 1 {
		domain := domainutil.ExpandShortName(args[0])
		if context.IsCloudDomain(domain) {
			cmd.SilenceUsage = true
			return cloudSwitch(domain, astroV1Client, out)
		}
	}
	return context.SwitchContext(cmd, args)
}

func newContextDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [domain]",
		Aliases: []string{"de"},
		Short:   "Delete a context",
		Long:    "Delete a locally stored context to Astro or APC",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.DeleteContext(cmd, args, noPrompt)
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().BoolVarP(&noPrompt, "force", "f", false, "Don't prompt a user before context delete; assume \"yes\" as answer to all prompts and run non-interactively.")
	return cmd
}
