package cloud

import (
	"io"

	airflow "github.com/astronomer/astro-cli/airflow-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/spf13/cobra"
)

var (
	astroCoreClient    astrocore.CoreClient
	astroCoreIamClient astroiamcore.CoreClient
	platformCoreClient astroplatformcore.CoreClient
	airflowAPIClient   airflow.Client
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(astroPlatformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, airflowClient airflow.Client, iamCoreClient astroiamcore.CoreClient, out io.Writer) []*cobra.Command {
	astroCoreClient = coreClient
	platformCoreClient = astroPlatformCoreClient
	astroCoreIamClient = iamCoreClient
	airflowAPIClient = airflowClient
	return []*cobra.Command{
		NewDeployCmd(),
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newDbtCmd(),
	}
}
