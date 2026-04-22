package cloud

import (
	"io"

	"github.com/spf13/cobra"

	airflow "github.com/astronomer/astro-cli/airflow-client"
	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
)

var (
	astroV1Client       astrov1.APIClient
	astroV1Alpha1Client astrov1alpha1.APIClient
	airflowAPIClient    airflow.Client
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(v1Client astrov1.APIClient, airflowClient airflow.Client, v1Alpha1Client astrov1alpha1.APIClient, out io.Writer) []*cobra.Command {
	astroV1Client = v1Client
	astroV1Alpha1Client = v1Alpha1Client
	airflowAPIClient = airflowClient
	return []*cobra.Command{
		NewDeployCmd(),
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newDbtCmd(),
		newIDECommand(out),
		newRemoteRootCmd(),
	}
}
