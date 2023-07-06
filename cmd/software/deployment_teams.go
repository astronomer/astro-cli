package software

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/software/deployment"
)

var deploymentRole string

const (
	deploymentTeamAddExample = `
	# Add a workspace team to a deployment with a particular role
	$ astro deployment team add --deployment-id=xxxxx --team-id=<team-id> --role=DEPLOYMENT_ROLE
	`
	deploymentTeamRemoveExample = `
	# Remove team access to a deployment
	$ astro deployment team remove <team-id> --deployment-id=xxxxx
	`
	deploymentTeamUpdateExample = `
	# Update a workspace team's deployment role
	$ astro deployment team update <team-id> --deployment-id=xxxxx --role=DEPLOYMENT_ROLE
	`
	deploymentTeamsListExample = `
	# List all teams added to a deployment
	$ astro deployment teams list <deployment-id>`
)

func newDeploymentTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage deployment team resources",
		Long:    "A Team is a group of users imported from your Identity Provider, teams can be added to and removed from a deployment to manage group user access",
	}
	_ = cmd.MarkFlagRequired("deployment-id")
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment to associate team to")
	cmd.AddCommand(
		newDeploymentTeamListCmd(out),
		newDeploymentTeamAddCmd(out),
		newDeploymentTeamRemoveCmd(out),
		newDeploymentTeamUpdateCmd(out),
	)
	return cmd
}

func newDeploymentTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a team to a deployment",
		Long:    "Add a team to a deployment",
		Example: deploymentTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamAdd(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "team to be added to deployment")
	_ = cmd.MarkFlagRequired("team-id")
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewerRole, "deployment role assigned to team")
	return cmd
}

func newDeploymentTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove TEAM",
		Short:   "Remove a team from a deployment",
		Long:    "Remove a team from a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentTeamRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamRemove(cmd, out, args)
		},
	}
	return cmd
}

func newDeploymentTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update TEAM",
		Short:   "Update a team's role for a deployment",
		Long:    "Update a team's role for a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentTeamUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewerRole, "role assigned to team")
	return cmd
}

func newDeploymentTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Teams inside an Astronomer Deployment",
		Long:    "List Teams inside an Astronomer Deployment",
		Example: deploymentTeamsListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamsList(cmd, out, args)
		},
	}
	return cmd
}

func deploymentTeamsList(cmd *cobra.Command, out io.Writer, _ []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.ListTeamRoles(deploymentID, houstonClient, out)
}

func deploymentTeamAdd(cmd *cobra.Command, out io.Writer, _ []string) error {
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.AddTeam(deploymentID, teamID, deploymentRole, houstonClient, out)
}

func deploymentTeamRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.RemoveTeam(deploymentID, args[0], houstonClient, out)
}

func deploymentTeamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UpdateTeamRole(deploymentID, args[0], deploymentRole, houstonClient, out)
}
