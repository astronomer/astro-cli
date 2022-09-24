package cloud

import (
	"io"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/astro-client/generated/golang/astropublicapi"

	"github.com/spf13/cobra"
)

var (
	astroClient      astro.Client
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(client astro.Client, out io.Writer) []*cobra.Command {
	astroClient = client
	return []*cobra.Command{
		newDeployCmd(),
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newUserCmd(out),
	}
}
