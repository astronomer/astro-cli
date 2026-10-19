package deploy

import (
	"context"
	"errors"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// agentEnvVarPrefix is the Remote Execution agent's settings namespace, which
// its image sets variables in and an Astro Runtime image does not: the agent
// image is built FROM a Runtime image and adds no label of its own, so its ENV
// lines are what tell the two apart (astro-agent's
// packages/astro-agent-client/image/Dockerfile*). The namespace, not one
// setting in it, so renaming a setting does not blind the check.
const agentEnvVarPrefix = "ASTRO_AGENT_CLIENT_"

// errAgentImageInHostedDeploy refuses a Remote Execution agent image on the
// hosted deploy, where it would run as the Deployment's scheduler and
// crash-loop: the agent's entrypoint runs only its own components.
var errAgentImageInHostedDeploy = errors.New("the image to deploy is a Remote Execution agent image (it sets " + agentEnvVarPrefix + "* environment variables), which cannot run this Deployment's scheduler. Deploy an agent image with 'astro remote deploy'; for this Deployment, build FROM an Astro Runtime image, not the astro-remote-execution-agent one. If it is not an agent image, deploy it with --skip-agent-image-check")

// refuseAgentImage is the hosted deploy's check, on the image it ships: an
// agent image passes the Runtime label check, since it is built FROM a
// Runtime image. A failed inspect is not a verdict, so it lets the deploy go
// on.
//
// The check reads environment variable names, so an image that is not an
// agent's can still trip it, and the user needs a way past: the refusal
// names --skip-agent-image-check, which skips it for one deploy. show_warnings
// false skips it too, as it did in Astro CLI 1.x.
func refuseAgentImage(ctx context.Context, engine imagebuild.Engine, image string) error {
	if !config.CFG.ShowWarnings.GetBool() {
		logger.Debugf("skipping the agent image check: show_warnings is false")
		return nil
	}
	isAgent, err := engine.HasEnvPrefix(ctx, image, agentEnvVarPrefix)
	if err != nil {
		logger.Debugf("skipping the agent image check: %s", err)
		return nil
	}
	if isAgent {
		return errAgentImageInHostedDeploy
	}
	return nil
}
