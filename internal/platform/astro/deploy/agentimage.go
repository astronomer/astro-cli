package deploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// agentEnvVarPrefix starts an environment variable that the Remote Execution
// agent image sets and an Astro Runtime image does not: the agent image is
// built FROM a Runtime image and adds no label of its own, so its ENV lines
// are what tell the two apart (astro-agent's
// packages/astro-agent-client/image/Dockerfile*).
const agentEnvVarPrefix = "ASTRO_AGENT_CLIENT_PROCESS_SPAWNER"

// errAgentImageInHostedDeploy refuses a Remote Execution agent image on the
// hosted deploy, where it would run as the Deployment's scheduler and
// crash-loop: the agent's entrypoint runs only its own components.
var errAgentImageInHostedDeploy = errors.New("the image to deploy is a Remote Execution agent image (it sets " + agentEnvVarPrefix + "* environment variables), which cannot run this Deployment's scheduler. Build it FROM an Astro Runtime image, or deploy an agent image with 'astro remote deploy'")

// isAgentImage reports whether the image sets agentEnvVarPrefix, and whether
// it could tell: a failed inspect is not a verdict either way, so neither
// check acts on one.
func isAgentImage(ctx context.Context, engine imagebuild.Engine, image string) (isAgent, known bool) {
	found, err := engine.HasEnvPrefix(ctx, image, agentEnvVarPrefix)
	if err != nil {
		logger.Debugf("skipping the agent image check: %s", err)
		return false, false
	}
	return found, true
}

// refuseAgentImage is the hosted deploy's check, on the image it ships: an
// agent image passes the Runtime label check, since it is built FROM a
// Runtime image.
func refuseAgentImage(ctx context.Context, engine imagebuild.Engine, image string) error {
	if isAgent, _ := isAgentImage(ctx, engine, image); isAgent {
		return errAgentImageInHostedDeploy
	}
	return nil
}

// warnIfNotAgentImage is the client deploy's check, and only a warning: a
// failed inspect says nothing either way, and the image may be one the user
// means to run as an agent all the same.
func warnIfNotAgentImage(ctx context.Context, engine imagebuild.Engine, image string) {
	if isAgent, known := isAgentImage(ctx, engine, image); known && !isAgent {
		fmt.Printf("WARNING: The client image does not look like a Remote Execution agent image: it sets no %s* environment variable. Check that Dockerfile.client builds FROM an astro-remote-execution-agent image.\n", agentEnvVarPrefix)
	}
}
