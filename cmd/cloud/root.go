package cloud

import (
	"io"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/spf13/cobra"
)

var (
	astroClient     astro.Client
	astroCoreClient astrocore.CoreClient
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(client astro.Client, coreClient astrocore.CoreClient, out io.Writer) []*cobra.Command {
	astroClient = client
	astroCoreClient = coreClient
	return []*cobra.Command{
		newDeployCmd(),
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newUserCmd(out),
	}
}
