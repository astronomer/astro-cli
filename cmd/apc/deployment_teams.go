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
  astro deployment team add <TEAM_ID> --deployment=<DEPLOYMENT_ID>

  # Give it another role
  astro deployment team add <TEAM_ID> --deployment=<DEPLOYMENT_ID> \
    --role=DEPLOYMENT_EDITOR`
	deploymentTeamRemoveExample = `  # Remove a team's access to a Deployment
  astro deployment team remove <TEAM_ID> --deployment=<DEPLOYMENT_ID>`
	deploymentTeamUpdateExample = `  # Change a team's role in a Deployment
  astro deployment team update <TEAM_ID> --deployment=<DEPLOYMENT_ID> --role=DEPLOYMENT_ADMIN`
	deploymentTeamsListExample = `  # List the teams in a Deployment
  astro deployment team list --deployment=<DEPLOYMENT_ID>`
)

func newDeploymentTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage Deployment team resources",
		Long:    "A Team is a group of users imported from your Identity Provider. Add a Team to or remove it from a Deployment to manage access for the whole group.",
	}
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose teams to manage: a link name from the project's manifest, or a Deployment id", "d", true)
	// Every team command needs it. Marked before the flag was defined, as it
	// was, the mark failed, unseen, and a run without one asked Houston about
	// the Deployment "".
	_ = cmd.MarkPersistentFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.AddCommand(
		newDeploymentTeamListCmd(out),
		newDeploymentTeamAddCmd(out),
		newDeploymentTeamRemoveCmd(out),
		newDeploymentTeamUpdateCmd(out),
	)
	return cmd
}

func newDeploymentTeamAddCmd(out io.Writer) *cobra.Command { //nolint:dupl // the Workspace twin differs in its flags and its role
	cmd := cliout.CheckArgs(cliout.Args(&cobra.Command{
		Use:     "add",
		Short:   "Add a team to a Deployment",
		Long:    "Add a team to a Deployment, named by its id as the argument or with --team-id",
		Example: deploymentTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamAdd(out, args)
		},
	}, cliout.Optional("TEAM_ID")), argOrFlag("team-id", "team"))
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "Team to add to the Deployment")
	// A persistent flag is not among the command's own flags, where
	// MarkFlagRequired looks, so marking it so failed, unseen, and a run
	// without one asked Houston about the team "".
	_ = cmd.MarkPersistentFlagRequired("team-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewerRole, "Deployment role to give the team: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR or DEPLOYMENT_ADMIN")
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a team from a Deployment",
		Long:    "Remove a team from a Deployment",
		Example: deploymentTeamRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamRemove(out, args)
		},
	}, cliout.Required("TEAM_ID"))
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Short:   "Update a team's role for a Deployment",
		Long:    "Update a team's role for a Deployment",
		Example: deploymentTeamUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentTeamUpdate(out, args)
		},
	}, cliout.Required("TEAM_ID"))
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
			return deploymentTeamsList(out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func deploymentTeamsList(out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
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

func deploymentTeamAdd(out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("could not find a valid role: %w", err)
	}

	role, err := deployment.AddTeam(deploymentID, teamID, deploymentRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentTeamChange(r, deploymentID, teamID, role, true)
}

func deploymentTeamRemove(out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := deployment.RemoveTeam(deploymentID, args[0], houstonClient); err != nil {
		return err
	}
	return renderDeploymentTeamRemoval(r, deploymentID, args[0])
}

func deploymentTeamUpdate(out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("could not find a valid role: %w", err)
	}

	role, err := deployment.UpdateTeamRole(deploymentID, args[0], deploymentRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentTeamChange(r, deploymentID, args[0], role, false)
}
