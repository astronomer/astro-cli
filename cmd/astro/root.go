package astro

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

var (
	astroV1Client       astrov1.APIClient
	astroV1Alpha1Client astrov1alpha1.APIClient
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(v1Client astrov1.APIClient, v1Alpha1Client astrov1alpha1.APIClient, out io.Writer) []*cobra.Command {
	astroV1Client = v1Client
	astroV1Alpha1Client = v1Alpha1Client
	return []*cobra.Command{
		NewDeployCmd(),
		newDeploymentRootCmd(out),
		newEnvRootCmd(out),
		newWorkspaceCmd(out),
		newOrganizationCmd(out),
		newDbtCmd(),
		newIDECommand(out),
		newRemoteRootCmd(),
	}
}
