package astro

// Rendering for astro deploy agent. The platform package returns what
// happened; this file decides how it looks, in text and in json.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

// deployAgentJSON is what `astro deploy agent --output json` publishes: the
// agent image it pushed, which is what the agents' Helm values then name.
type deployAgentJSON struct {
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
	RuntimeCheck *agentRuntimeCheckJSON `json:"runtime_check,omitempty"`
}

type agentRuntimeCheckJSON struct {
	Deployment               string `json:"deployment"`
	AgentRuntimeVersion      string `json:"agent_runtime_version"`
	DeploymentRuntimeVersion string `json:"deployment_runtime_version"`
}

func newDeployAgentJSON(res *astrodeploy.AgentDeploy) deployAgentJSON {
	obj := deployAgentJSON{
		Image:       res.Image,
		Registry:    res.Registry,
		Tag:         res.Tag,
		SourceImage: res.SourceImage,
		Platforms:   nonNil(res.Platforms),
	}
	if c := res.RuntimeCheck; c != nil {
		obj.RuntimeCheck = &agentRuntimeCheckJSON{
			Deployment:               c.DeploymentID,
			AgentRuntimeVersion:      c.AgentRuntimeVersion,
			DeploymentRuntimeVersion: c.DeploymentRuntimeVersion,
		}
	}
	return obj
}

// renderDeployAgent is the text a finished agent image push prints: where it
// went, and what to do with it next.
func renderDeployAgent(image string) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Pushed agent image to %s\n", ansi.Bold(image))
		fmt.Fprintf(b, "Next, set image: in your agent Helm chart's values.yaml to %s and run helm upgrade, or have your CI/CD pipeline do it.\n", image)
	})
}

// nonNil is s, or [] for nil, so a list publishes as [] and never null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
