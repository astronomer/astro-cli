package astro

import (
	"errors"
	"fmt"
	"io"
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
	remotePlatform     string
	remoteImageName    string
	remoteBuildSecrets = []string{}
	remoteDeploymentID string
	remoteRegistry     string
)

const (
	remoteDeployExample = `  # Build this project's agent image and push it to the remote registry
  astro remote deploy

  # Build it for several platforms
  astro remote deploy --platform linux/amd64,linux/arm64

  # Push it to the registry a linked Deployment's remote agents pull from
  astro remote deploy --deployment prod

  # Push an image already built on this machine, checking its runtime against a Deployment's
  astro remote deploy --image-name <IMAGE_NAME> --registry <REGISTRY>/<REPOSITORY> \
    --deployment <DEPLOYMENT_ID>`
)

// newRemoteRootCmd creates the root command for remote operations
func newRemoteRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Push the image Remote Execution agents run",
		Long:  "Commands for Remote Execution: building the agent image a Deployment's agents run in your cluster, and pushing it to the registry they pull from.",
	}
	// The results leave by cmd.OutOrStdout(), which must be the stdout the
	// tree was built with: under json strayStdoutToStderr points os.Stdout
	// at stderr, and an unbound command would follow it there.
	cmd.SetOut(out)

	cmd.AddCommand(newRemoteDeployCmd())
	applyPreferredFlagsIn(cmd)
	return cmd
}

// newRemoteDeployCmd creates the remote deploy command
func newRemoteDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build the agent image and push it to the agents' registry",
		Long:  "Build this project's agent image from Dockerfile.client and push it to the registry a Remote Execution Deployment's agents pull from. This command assumes you have already authenticated with the registry.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// The rule astro deploy follows: a build needs a pyproject.toml
			// project's root, and in or below a project in the Astro CLI 1.x
			// layout, or below a pyproject.toml project's root, everything is
			// refused with the same advice. --image-name builds nothing, so
			// outside any project it pushes the image as it is. The value,
			// not whether the flag was given: an empty --image-name= names
			// no image.
			where := utils.Locate(config.WorkingPath)
			if where.AtRoot(config.WorkingPath) || (remoteImageName != "" && where.None()) {
				return nil
			}
			return utils.NoDeployableProject(where)
		},
		RunE:    remoteDeploy,
		Example: remoteDeployExample,
	}

	cmd.Flags().StringVar(&remotePlatform, "platform", "", "Target platform for agent image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVar(&remoteImageName, "image-name", "", "Name of a custom image to deploy, or image name with custom tag; the image must be present on this machine")
	utils.AddBuildSecretFlag(cmd.Flags(), &remoteBuildSecrets)
	cmd.Flags().StringVar(&remoteRegistry, "registry", "", "Repository to push the agent image to, registry[:port]/repository with no tag: the one the Deployment's remote agents pull from. Default: the linked Deployment's agent-registry, else [tool.astro.deploy] agent-registry")
	addDeploymentFlag(cmd.Flags(), "Deployment whose runtime version the agent image is validated against: a link name of this project, or a Deployment id. A link's agent-registry is the one pushed to", deploymentTargets{id: &remoteDeploymentID}, oneXDeploymentID)
	cliout.AddOutputFlag(cmd, &remoteOutput)

	return cmd
}

// remoteOutput is --output for `astro remote deploy`.
var remoteOutput cliout.Format

// deployAgentImage builds and pushes the agent image. A var so a test can
// stand in for Docker and the registry.
var deployAgentImage = astrodeploy.DeployAgentImage

// remoteDeploy handles the remote deploy functionality
func remoteDeploy(cmd *cobra.Command, args []string) error {
	// The build's and the push's own output, and the notes on the way, are
	// what text mode has always shown; under json they go to stderr.
	defer strayStdoutToStderr(remoteOutput)()

	target, err := resolveAgentTarget(manifestHere(), remoteRegistry, remoteDeploymentID)
	if err != nil {
		return err
	}
	deployInput := astrodeploy.InputAgentDeploy{
		Path:           config.WorkingPath,
		ImageName:      remoteImageName,
		Platform:       remotePlatform,
		BuildSecrets:   util.ResolveBuildSecrets(remoteBuildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		DeploymentID:   target.deploymentID,
		Registry:       target.registry,
		RegistrySource: target.source,
		ErrOut:         cmd.ErrOrStderr(),
		Context:        cmd.Context(),
	}

	res, err := deployAgentImage(deployInput, astroV1Client)
	if err != nil {
		return err
	}
	return cliout.Renderer{Format: remoteOutput, Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}.Emit(newRemoteDeployJSON(&res), renderRemoteDeploy(res.Image))
}

// projectAgentRegistry names [tool.astro.deploy] agent-registry as the
// source of a registry, for an error about its value.
const projectAgentRegistry = "pyproject.toml [tool.astro.deploy] agent-registry"

// agentTarget is where `astro remote deploy` pushes, and the Deployment it
// checks the image's runtime against.
type agentTarget struct {
	registry, source, deploymentID string
}

// resolveAgentTarget decides where the agent image goes: --registry, else
// the agent-registry of the link --deployment names, else the project's
// [tool.astro.deploy] agent-registry. The registry is the one the
// Deployment's remote agents pull from, so it is the project's to commit,
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
				return agentTarget{}, fmt.Errorf("link %s is %s link: only an Astro Deployment runs remote agents", deployment, manifest.KindWithArticle(l.Kind()))
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
		return agentTarget{}, cliout.Usage(errors.New("no agent registry: pass --registry <registry>/<repository>, the repository the Deployment's remote agents pull the agent image from. Outside a project it is the only way to name one"))
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
		return agentTarget{}, errors.New("no agent registry: set agent-registry under [tool.astro.deploy] in pyproject.toml to the repository the project's remote agents pull the agent image from, so every checkout pushes there, or pass --registry for this run")
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
