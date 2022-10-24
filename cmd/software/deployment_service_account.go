package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/software/service_account"
	"github.com/spf13/cobra"
)

var (
	deploymentSACreateLabel    string
	deploymentSACreateCategory string
	deploymentSACreateRole     string

	deploymentSaCreateExample = `
# Create service-account
  $ astro deployment service-account create --deployment-id=xxxxx --label=my_label --role=ROLE
`
	deploymentSaListExample = `
  # Get deployment service-accounts
  $ astro deployment service-account list --deployment-id=<deployment-id>
`
	deploymentSaDeleteExample = `
  $ astro deployment service-account delete <service-account-id> --deployment-id=<deployment-id>
`
)

func newDeploymentSaRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage astronomer service accounts",
		Long:    "Service-accounts represent a revokable token with access to the Astronomer platform",
	}
	cmd.AddCommand(
		newDeploymentSaCreateCmd(out),
		newDeploymentSaListCmd(out),
		newDeploymentSaDeleteCmd(out),
	)
	return cmd
}

//nolint:dupl
func newDeploymentSaCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a service-account in the astronomer platform",
		Long:    "Create a service-account in the astronomer platform",
		Example: deploymentSaCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment in which you wish to manage Service Accounts")
	cmd.Flags().StringVarP(&deploymentSACreateCategory, "category", "c", "default", "Category of the Service Account")
	cmd.Flags().StringVarP(&deploymentSACreateLabel, "label", "l", "", "Label of the Service Account")
	cmd.Flags().StringVarP(&deploymentSACreateRole, "role", "r", houston.DeploymentViewerRole, "Role of the Service Account to create, one of: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR, DEPLOYMENT_ADMIN")
	_ = cmd.MarkFlagRequired("label")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func newDeploymentSaListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Service Accounts inside a deployment",
		Long:    "List Service Accounts inside a deployment",
		Example: deploymentSaListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment in which you wish to manage Service Accounts")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func newDeploymentSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [service-account ID]",
		Aliases: []string{"de"},
		Short:   "Delete a service-account in the astronomer platform",
		Long:    "Delete a service-account in the astronomer platform",
		Example: deploymentSaDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaDelete(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment in which you wish to manage Service Accounts")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func deploymentSaCreate(cmd *cobra.Command, out io.Writer) error {
	if err := validateDeploymentRole(deploymentSACreateRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingDeploymentUUID(deploymentID, deploymentSACreateLabel, deploymentSACreateCategory, deploymentSACreateRole, houstonClient, out)
}

func deploymentSaList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.GetDeploymentServiceAccounts(deploymentID, houstonClient, out)
}

func deploymentSaDelete(cmd *cobra.Command, args []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingDeploymentUUID(args[0], deploymentID, houstonClient, out)
}
