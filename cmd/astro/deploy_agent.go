package astro

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	agentPlatform     string
	agentImageName    string
	agentBuildSecrets = []string{}
	agentDeploymentID string
	agentRegistry     string
)

const (
	deployAgentExample = `  # Build this project's agent image and push it to the agent registry
  astro deploy agent

  # Build it for several platforms
  astro deploy agent --platform linux/amd64,linux/arm64

  # Push it to the registry a linked Deployment's agents pull from
  astro deploy agent --deployment prod

  # Push an image already built on this machine, checking its runtime against a Deployment's
  astro deploy agent --image-name <IMAGE_NAME> --registry <REGISTRY>/<REPOSITORY> \
    --deployment <DEPLOYMENT_ID>`

	// agentCommand is the deploy subcommand that pushes the agent image.
	agentCommand = "agent"
)

// newRemoteRootCmd is astro remote, kept for Astro CLI 1.x's astro remote
// deploy, which is astro deploy agent now: hidden, with that one verb under
// it, also hidden, still running and saying what to type instead. The rest of
// the astro remote namespace is free for commands of its own.
func newRemoteRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "remote",
		Short:  "Run Astro CLI 1.x's remote deploy, now astro deploy agent",
		Hidden: true,
	}
	cmd.AddCommand(deprecatedRemoteDeploy(newDeployAgentCmd(oneXDeploymentID)))
	applyPreferredFlagsIn(cmd)
	return cmd
}

// deprecatedRemoteDeploy turns cmd, a fresh build of astro deploy agent with
// 1.x's flag spellings, into astro remote deploy: hidden, still pushing, and
// saying once per run that the spelling is deprecated. Its arguments are
// left unchecked, as 1.x left them.
func deprecatedRemoteDeploy(cmd *cobra.Command) *cobra.Command {
	cmd.Use, cmd.Args = "deploy", nil
	return cliout.DeprecateCommand(cmd, func(c *cobra.Command) string {
		return c.Root().Name() + " deploy " + agentCommand
	}, "")
}

// newDeployAgentCmd is astro deploy agent. oneX are the 1.x spellings of
// --deployment its flag accepts: none here, 1.x's --deployment-id on the
// astro remote deploy spelling.
//
// The word agent is a subcommand of astro deploy, as bundle is, so a link of
// that name is reached with --deployment, never as the Deployment argument.
func newDeployAgentCmd(oneX ...oneXSpelling) *cobra.Command {
	cmd := &cobra.Command{
		Use:   agentCommand,
		Short: "Push this project's Remote Execution agent image to its registry",
		Long:  "Build the project's Remote Execution agent image from its Dockerfile.client and push it to the registry the Deployment's agents pull from. Log in to that registry first. The agents run the new image once their Helm values name it.",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// The rule astro deploy follows: a build needs a pyproject.toml
			// project's root, and in or below a project in the Astro CLI 1.x
			// layout, or below a pyproject.toml project's root, everything is
			// refused with the same advice. --image-name builds nothing, so
			// outside any project it pushes the image as it is. The value,
			// not whether the flag was given: an empty --image-name= names
			// no image.
			where := utils.Locate(config.WorkingPath)
			if where.AtRoot(config.WorkingPath) || (agentImageName != "" && where.None()) {
				return nil
			}
			return utils.NoDeployableProject(where)
		},
		RunE:    deployAgent,
		Example: deployAgentExample,
	}

	cmd.Flags().StringVar(&agentPlatform, "platform", "", "Target platform for the agent image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVar(&agentImageName, "image-name", "", "Name of a custom image to deploy, or image name with custom tag; the image must be present on this machine")
	utils.AddBuildSecretFlag(cmd.Flags(), &agentBuildSecrets)
	cmd.Flags().StringVar(&agentRegistry, "registry", "", "Repository to push the agent image to, registry[:port]/repository with no tag: the one the Deployment's agents pull from. Default: the linked Deployment's agent-registry, else [tool.astro.deploy] agent-registry")
	addDeploymentFlag(cmd.Flags(), "Deployment whose runtime version the agent image is validated against: a link name of this project, or a Deployment id. A link's agent-registry is the one pushed to", deploymentTargets{id: &agentDeploymentID}, oneX...)
	cliout.AddOutputFlag(cmd, &agentOutput)

	return cmd
}

// agentOutput is --output for `astro deploy agent`.
var agentOutput cliout.Format

// deployAgentImage builds and pushes the agent image. A var so a test can
// stand in for Docker and the registry.
var deployAgentImage = astrodeploy.DeployAgentImage

// deployAgent builds and pushes the agent image, or pushes --image-name.
func deployAgent(cmd *cobra.Command, args []string) error {
	// The result goes to the stdout the command was given, read before the
	// build's and the push's own output, and the notes on the way, are sent
	// to stderr under json.
	out := cmd.OutOrStdout()
	defer strayStdoutToStderr(agentOutput)()

	target, err := resolveAgentTarget(manifestHere(), agentRegistry, agentDeploymentID)
	if err != nil {
		return err
	}
	deployInput := astrodeploy.InputAgentDeploy{
		Path:           config.WorkingPath,
		ImageName:      agentImageName,
		Platform:       agentPlatform,
		BuildSecrets:   util.ResolveBuildSecrets(agentBuildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		DeploymentID:   target.deploymentID,
		Registry:       target.registry,
		RegistrySource: target.source,
		ErrOut:         cmd.ErrOrStderr(),
	}

	res, err := deployAgentImage(deployInput, astroV1Client)
	if err != nil {
		return err
	}
	return cliout.Renderer{Format: agentOutput, Out: out}.Emit(newDeployAgentJSON(&res), renderDeployAgent(res.Image))
}

// projectAgentRegistry names [tool.astro.deploy] agent-registry as the
// source of a registry, for an error about its value.
const projectAgentRegistry = "pyproject.toml [tool.astro.deploy] agent-registry"

// agentTarget is where `astro deploy agent` pushes, and the Deployment it
// checks the image's runtime against.
type agentTarget struct {
	registry, source, deploymentID string
}

// resolveAgentTarget decides where the agent image goes: --registry, else
// the agent-registry of the link --deployment names, else the project's
// [tool.astro.deploy] agent-registry. The registry is the one the
// Deployment's agents pull from, so it is the project's to commit,
// for every teammate and CI job; --registry is for one run, and the only
// source outside a project (an --image-name push). source names where the
// value came from, for an error about it.
//
// --deployment may name a link of the project, as the deploys' does; the
// runtime check then reads the link's Deployment. With no --deployment, the
// project's default link (manifest.DefaultLink) gives its agent-registry, as
// it is the link the project's other commands fall through to, but not its
// Deployment: a run that named none has never been checked against one, and
// checking would call the API where it did not before.
func resolveAgentTarget(m *manifest.Manifest, registry, deployment string) (agentTarget, error) {
	t := agentTarget{registry: registry, source: "--registry", deploymentID: deployment}
	var link *manifest.Link
	linkName := deployment
	if m != nil && deployment != "" {
		if l, ok := m.Astro.Deployments[deployment]; ok {
			if l.Kind() != manifest.KindAstro {
				return agentTarget{}, fmt.Errorf("link %s is %s link: only an Astro Deployment runs Remote Execution agents", deployment, manifest.KindWithArticle(l.Kind()))
			}
			link = &l
			t.deploymentID = l.Deployment
		} else if linked, l, ok := astroLinkTo(m, deployment); ok {
			// A Deployment id the project links: its agents pull from that
			// link's registry, however the run named it.
			link, linkName = &l, linked
		}
	}
	if m != nil && deployment == "" {
		if defName, l, ok := manifest.DefaultLink(m.Astro.Deployments); ok && l.Kind() == manifest.KindAstro {
			link, linkName = &l, defName
		}
	}
	if t.registry != "" {
		return t, nil
	}
	switch {
	case m == nil:
		return agentTarget{}, cliout.Usage(errors.New("no agent registry: pass --registry <registry>/<repository>, the repository the Deployment's agents pull the agent image from. Outside a project it is the only way to name one"))
	case link != nil && link.AgentRegistry != "":
		t.registry = link.AgentRegistry
		t.source = projectAgentRegistry
		if link.AgentRegistry != m.Astro.Deploy.AgentRegistry {
			t.source = "pyproject.toml [tool.astro.deployments." + linkName + "] agent-registry"
		}
	case m.Astro.Deploy.AgentRegistry != "":
		t.registry = m.Astro.Deploy.AgentRegistry
		t.source = projectAgentRegistry
	default:
		return agentTarget{}, errors.New("no agent registry: set agent-registry under [tool.astro.deploy] in pyproject.toml to the repository the project's agents pull the agent image from, so every checkout pushes there, or pass --registry for this run")
	}
	return t, nil
}

// astroLinkTo is the project's astro link to the Deployment id, if one links
// it. Map order is not stable, so of two links to one Deployment the
// lowest-named wins.
func astroLinkTo(m *manifest.Manifest, id string) (name string, link manifest.Link, ok bool) {
	for n := range m.Astro.Deployments {
		l := m.Astro.Deployments[n]
		if l.Kind() == manifest.KindAstro && l.Deployment == id && (!ok || n < name) {
			name, link, ok = n, l, true
		}
	}
	return name, link, ok
}
