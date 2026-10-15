package astro

// Rendering for astro remote deploy. The platform package returns what
// happened; this file decides how it looks, in text and in json.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

// remoteDeployJSON is what `astro remote deploy --output json` publishes: the
// client image it pushed, which is what the agents' Helm values then name.
type remoteDeployJSON struct {
	Image    string `json:"image"`
	Registry string `json:"registry"`
	Tag      string `json:"tag"`
	// SourceImage is the local image pushed (--image-name), omitted when the
	// run built one.
	SourceImage string `json:"source_image,omitempty"`
	// Platforms are the --platform the image was built for, [] for the host's
	// or when nothing was built.
	Platforms []string `json:"platforms"`
	// RuntimeCheck is the check against --deployment, omitted without one.
	RuntimeCheck *remoteRuntimeCheckJSON `json:"runtime_check,omitempty"`
}

type remoteRuntimeCheckJSON struct {
	Deployment               string `json:"deployment"`
	ClientRuntimeVersion     string `json:"client_runtime_version"`
	DeploymentRuntimeVersion string `json:"deployment_runtime_version"`
}

func newRemoteDeployJSON(res *astrodeploy.ClientDeploy) remoteDeployJSON {
	obj := remoteDeployJSON{
		Image:       res.Image,
		Registry:    res.Registry,
		Tag:         res.Tag,
		SourceImage: res.SourceImage,
		Platforms:   nonNil(res.Platforms),
	}
	if c := res.RuntimeCheck; c != nil {
		obj.RuntimeCheck = &remoteRuntimeCheckJSON{
			Deployment:               c.DeploymentID,
			ClientRuntimeVersion:     c.ClientRuntimeVersion,
			DeploymentRuntimeVersion: c.DeploymentRuntimeVersion,
		}
	}
	return obj
}

// renderRemoteDeploy is the text a finished client image push has always
// printed: where it went, and what to do with it next.
func renderRemoteDeploy(image string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully pushed client image to %s\n", ansi.Bold(image))
		fmt.Fprintf(b, "\n--------------------------------\n")
		fmt.Fprintln(b, "The client image has been pushed to your private registry.")
		fmt.Fprintln(b, "Your next step would be to update the agent component to use the new client image.")
		fmt.Fprintln(b, "For that you would either need to update the helm chart values.yaml file or update your CI/CD pipeline to use the new client image.")
		fmt.Fprintf(b, "If you are using Astronomer provided Agent Helm chart, you would need to update the image field for each of the workers, dagProcessor, and triggerer component sections to the new image: %s\n", image)
		fmt.Fprintln(b, "Once you have updated the helm chart values.yaml file, you can run 'helm upgrade' or update via your CI/CD pipeline to update the agent components")
	})
}

// nonNil is s, or [] for nil, so a list publishes as [] and never null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
