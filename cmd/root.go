package cmd

import (
	"io"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	workspaceId   string
	workspaceRole string
	role          string
)

func persistentPreRun(cmd *cobra.Command, out io.Writer, args []string) {
	l := "v1.20.9" // TODO: API Request here
	t := "^" + l[:strings.LastIndex(l, ".")]
	p, err := semver.NewConstraint(t)
	if err != nil {
		// Handle constraint not being parsable.
		color.Red("Error with %s", err)
	}

	v, err := semver.NewVersion(version.CurrVersion)
	if err != nil {
		color.Red("Error with %s", err)
	}

	if p.Check(v) {
		color.Yellow("A new patch is available. Your version is %s and %s is the latest", v, l)
	} else {
		color.Red("There is an update for astro-cli. You're using %s and %s is the latest", v, l)
	}
}

func NewRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			persistentPreRun(cmd, out, args)
		},
	}

	rootCmd.AddCommand(
		newAuthRootCmd(client, out),
		newWorkspaceCmd(client, out),
		newVersionCmd(out),
		newUpgradeCheckCmd(out),
		newUserCmd(client, out),
		newClusterRootCmd(client, out),
		newDevRootCmd(client, out),
		newCompletionCmd(client, out),
		newConfigRootCmd(client, out),
		newDeploymentRootCmd(client, out),
		newDeployCmd(client, out),
		newSaRootCmd(client, out),
		// TODO: remove newAirflowRootCmd, after 1.0 we have only devRootCmd
		newAirflowRootCmd(client, out),
		newLogsDeprecatedCmd(client, out),
	)
	return rootCmd
}
