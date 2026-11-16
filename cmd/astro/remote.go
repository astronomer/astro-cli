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
	remoteDeployExample = `  # Build this project's client image and push it to the remote registry
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
		Short: "Manage remote deploys and images",
		Long:  "Commands for interacting with remote registries and deploying client images",
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
		Short: "Deploy a client image to the remote registry",
		Long:  "Build and deploy a client image to the configured remote registry. This command assumes you have already authenticated with the registry.",
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
			cmd.SilenceUsage = true
			return utils.NoDeployableProject(where)
		},
		RunE:    remoteDeploy,
		Example: remoteDeployExample,
	}

	cmd.Flags().StringVar(&remotePlatform, "platform", "", "Target platform for client image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVarP(&remoteImageName, "image-name", "i", "", "Name of a custom image to deploy, or image name with custom tag; the image must be present on this machine")
	utils.AddBuildSecretFlag(cmd.Flags(), &remoteBuildSecrets)
	cmd.Flags().StringVar(&remoteRegistry, "registry", "", "Repository to push the client image to, registry[:port]/repository with no tag: the one the Deployment's remote agents pull from. Default: the linked Deployment's client-registry, else [tool.astro.deploy] client-registry")
	addDeploymentFlag(cmd.Flags(), "Deployment whose runtime version the client image is validated against: a link name of this project, or a Deployment id. A link's client-registry is the one pushed to", deploymentTargets{id: &remoteDeploymentID}, oneXDeploymentID(""))
	cliout.AddOutputFlag(cmd, &remoteOutput)

	return cmd
}

// remoteOutput is --output for `astro remote deploy`.
var remoteOutput cliout.Format

// deployClientImage builds and pushes the client image. A var so a test can
// stand in for Docker and the registry.
var deployClientImage = astrodeploy.DeployClientImage

// remoteDeploy handles the remote deploy functionality
func remoteDeploy(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	// The build's and the push's own output, and the notes on the way, are
	// what text mode has always shown; under json they go to stderr.
	defer strayStdoutToStderr(remoteOutput)()

	target, err := resolveClientTarget(manifestHere(), remoteRegistry, remoteDeploymentID)
	if err != nil {
		return err
	}
	deployInput := astrodeploy.InputClientDeploy{
		Path:           config.WorkingPath,
		ImageName:      remoteImageName,
		Platform:       remotePlatform,
		BuildSecrets:   util.ResolveBuildSecrets(remoteBuildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		DeploymentID:   target.deploymentID,
		Registry:       target.registry,
		RegistrySource: target.source,
		ErrOut:         cmd.ErrOrStderr(),
	}

	res, err := deployClientImage(deployInput, astroV1Client)
	if err != nil {
		return err
	}
	return cliout.Renderer{Format: remoteOutput, Out: cmd.OutOrStdout()}.Emit(newRemoteDeployJSON(&res), renderRemoteDeploy(res.Image))
}

// projectClientRegistry names [tool.astro.deploy] client-registry as the
// source of a registry, for an error about its value.
const projectClientRegistry = "pyproject.toml [tool.astro.deploy] client-registry"

// clientTarget is where `astro remote deploy` pushes, and the Deployment it
// checks the image's runtime against.
type clientTarget struct {
	registry, source, deploymentID string
}

// resolveClientTarget decides where the client image goes: --registry, else
// the client-registry of the link --deployment names, else the project's
// [tool.astro.deploy] client-registry. The registry is the one the
// Deployment's remote agents pull from, so it is the project's to commit,
// for every teammate and CI job; --registry is for one run, and the only
// source outside a project (an --image-name push). source names where the
// value came from, for an error about it.
//
// --deployment may name a link of the project, as the deploys' does; the
// runtime check then reads the link's Deployment.
func resolveClientTarget(m *manifest.Manifest, registry, deployment string) (clientTarget, error) {
	t := clientTarget{registry: registry, source: "--registry", deploymentID: deployment}
	var link *manifest.Link
	if m != nil && deployment != "" {
		if l, ok := m.Astro.Deployments[deployment]; ok {
			if l.Kind() != manifest.KindAstro {
				return clientTarget{}, fmt.Errorf("link %s is %s link: only an Astro Deployment runs remote agents", deployment, manifest.KindWithArticle(l.Kind()))
			}
			link = &l
			t.deploymentID = l.Deployment
		}
	}
	if t.registry != "" {
		return t, nil
	}
	switch {
	case m == nil:
		return clientTarget{}, errors.New("no client registry: pass --registry <registry>/<repository>, the repository the Deployment's remote agents pull the client image from. Outside a project it is the only way to name one")
	case link != nil && link.ClientRegistry != "":
		t.registry = link.ClientRegistry
		t.source = projectClientRegistry
		if link.ClientRegistry != m.Astro.Deploy.ClientRegistry {
			t.source = "pyproject.toml [tool.astro.deployments." + deployment + "] client-registry"
		}
	case m.Astro.Deploy.ClientRegistry != "":
		t.registry = m.Astro.Deploy.ClientRegistry
		t.source = projectClientRegistry
	default:
		return clientTarget{}, errors.New("no client registry: set client-registry under [tool.astro.deploy] in pyproject.toml to the repository the project's remote agents pull the client image from, so every checkout pushes there, or pass --registry for this run")
	}
	return t, nil
}
