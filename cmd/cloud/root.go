package cloud

import (
	"io"

	astro "github.com/astronomer/astro-cli/astro-client"
	astroPublicApi "github.com/astronomer/astro/apps/core-api-bindings/golang/public"

	"github.com/spf13/cobra"
)

var (
	astroClient      astro.Client
	publicRESTClient *astroPublicApi.APIClient
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(client astro.Client, prc *astroPublicApi.APIClient, out io.Writer) []*cobra.Command {
	astroClient = client
	publicRESTClient = prc
	return []*cobra.Command{
		newDeployCmd(),
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newUserCmd(out),
	}
}
