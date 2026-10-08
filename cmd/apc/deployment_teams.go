package apc

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

var deploymentRole string

const (
	deploymentTeamAddExample = `  # Give a Workspace team viewer access to a Deployment
  astro deployment team add --deployment-id=<DEPLOYMENT_ID> --team-id=<TEAM_ID>

  # Give it another role
  astro deployment team add --deployment-id=<DEPLOYMENT_ID> --team-id=<TEAM_ID> \
    --role=DEPLOYMENT_EDITOR`
	deploymentTeamRemoveExample = `  # Remove a team's access to a Deployment
  astro deployment team remove <TEAM_ID> --deployment-id=<DEPLOYMENT_ID>`
	deploymentTeamUpdateExample = `  # Change a team's role in a Deployment
  astro deployment team update <TEAM_ID> --deployment-id=<DEPLOYMENT_ID> --role=DEPLOYMENT_ADMIN`
	deploymentTeamsListExample = `  # List the teams in a Deployment
  astro deployment team list --deployment-id=<DEPLOYMENT_ID>`
)

func newDeploymentTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage deployment team resources",
		Long:    "A Team is a group of users imported from your Identity Provider, teams can be added to and removed from a deployment to manage group user access",
	}
	_ = cmd.MarkFlagRequired("deployment-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "Deployment whose teams to manage")
	cmd.AddCommand(
		newDeploymentTeamListCmd(out),
		newDeploymentTeamAddCmd(out),
		newDeploymentTeamRemoveCmd(out),
		newDeploymentTeamUpdateCmd(out),
	)
	return cmd
}

func newDeploymentTeamAddCmd(out io.Writer) *cobra.Command { //nolint:dupl // the Workspace twin differs in its flags and its role
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a team to a deployment",
		Long:    "Add a team to a deployment",
		Example: deploymentTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamAdd(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "Team to add to the Deployment")
	_ = cmd.MarkFlagRequired("team-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewerRole, "Deployment role to give the team: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR or DEPLOYMENT_ADMIN")
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <TEAM_ID>",
		Short:   "Remove a team from a deployment",
		Long:    "Remove a team from a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentTeamRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamRemove(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <TEAM_ID>",
		Short:   "Update a team's role for a deployment",
		Long:    "Update a team's role for a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentTeamUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewerRole, "Deployment role to give the team: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR or DEPLOYMENT_ADMIN")
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Teams inside an APC Deployment",
		Long:    "List Teams inside an APC Deployment",
		Example: deploymentTeamsListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamsList(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func deploymentTeamsList(cmd *cobra.Command, out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	ts, err := deployment.ListTeamRoles(deploymentID, houstonClient)
	// The text has always refused a Deployment Houston lists no team for.
	// A list is [] when it is empty, so json publishes that: Houston
	// answers [] for a Deployment that does not exist too, so the refusal
	// never told the two apart.
	if errors.Is(err, deployment.ErrNoDeploymentTeams) && r.Format == cliout.FormatJSON {
		ts, err = []deployment.TeamRole{}, nil
	}
	if err != nil {
		return err
	}
	return renderDeploymentTeamList(r, deploymentID, ts)
}

func deploymentTeamAdd(cmd *cobra.Command, out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	role, err := deployment.AddTeam(deploymentID, teamID, deploymentRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentTeamChange(r, deploymentID, teamID, role, true)
}

func deploymentTeamRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if err := deployment.RemoveTeam(deploymentID, args[0], houstonClient); err != nil {
		return err
	}
	return renderDeploymentTeamRemoval(r, deploymentID, args[0])
}

func deploymentTeamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	role, err := deployment.UpdateTeamRole(deploymentID, args[0], deploymentRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentTeamChange(r, deploymentID, args[0], role, false)
}
