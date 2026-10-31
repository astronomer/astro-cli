package apc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/containercfg"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/imageinstall"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/util"
)

// Seams a test replaces: the platform half of a pyproject.toml project's
// deploy, and whether this run can be asked which Deployment to ship to.
var (
	DeployManifestImage = deploy.DeployManifestImage

	stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

// deployManifest deploys the pyproject.toml project the working directory is
// the root of to an Astro Private Cloud Deployment: the image built from the
// manifest, or the prebuilt one --image-name names (on this machine, or with
// --remote already in a registry), with the DAGs as the Deployment takes
// them, or the DAGs alone (--dags). Which Deployment, and every rule about
// it, is internal/deploy's, which Astro's deploy runs too, fed the project's
// apc links on this context's domain; what only APC knows (Houston, its
// registry, its runtime list, how DAGs reach a Deployment) is
// internal/platform/apc/deploy's.
//
// root is nil for an --image-name deploy outside any project: there is no
// manifest, so the target is a Deployment id (the argument or --deployment)
// or the workspace's pick, and the image ships alone, with no DAGs.
func deployManifest(cmd *cobra.Command, args []string, root *utils.Where, out io.Writer) error {
	var m *manifest.Manifest
	if root != nil {
		if root.LoadErr != nil {
			return root.LoadErr
		}
		m = root.Manifest
	}
	// Flags that cannot go together are a usage mistake, said as APC's
	// deploy always said it, before anything is read or asked.
	if isDagOnlyDeploy && isImageOnlyDeploy {
		return cliout.Usage(ErrBothDagsOnlyAndImageOnlySet)
	}
	if err := refuseBuildSecret(cmd, m); err != nil {
		return err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	domain := manifest.NormalizeAPCDomain(c.Domain)

	linkName := ""
	if len(args) > 0 {
		linkName = args[0]
	}
	if description == "" {
		description = utils.GetDefaultDeployDescription(isDagOnlyDeploy)
	}

	// The deploy's progress goes to stdout in text, as APC's deploy always
	// has, and to stderr under json, where stdout carries the result. The
	// question goes to stderr either way.
	progress := cliout.NotesTo(cmd, deployOutput, out)
	errOut := cmd.ErrOrStderr()
	// Every warning goes to stderr, led by a yellow "Warning:", as Astro's
	// deploy does, and into the result's warnings for -o json; notes go to
	// stderr too. In text they print once, after the deploy's progress (the
	// build and the push), just above the result, or before the error a failed
	// deploy returns; the ones found before a question print before it
	// instead. Under json each prints as given.
	var warnings []string
	recap := cliout.NewRecap(errOut, deployOutput == cliout.FormatText)
	warnf := func(msg string) {
		warnings = append(warnings, msg)
		recap.Warn(msg)
	}
	interactive := deployOutput == cliout.FormatText && stdinIsTerminal()
	engine := utils.OnceEngine(func() (string, []string, error) { return containercfg.Engine(config.WorkingPath) })
	if runtimeVersionForImageName != "" && !imagePresentOnRemote {
		// Astro CLI 1.x took it here and read nothing of it, so it is still
		// taken, and said to do nothing.
		warnf("--runtime-version has no effect without --remote: the runtime of an image on this machine, or of one built, is read from its label")
	}

	res, err := manifestdeploy.Run(manifestdeploy.Request{
		ProjectDir:       config.WorkingPath,
		Manifest:         m,
		LinkName:         linkName,
		Deployment:       deployDeployment,
		WorkspaceID:      workspaceID,
		ContextWorkspace: c.Workspace,
		DagsOnly:         isDagOnlyDeploy,
		Image:            isImageOnlyDeploy,
		ImageName:        imageName,
		ImageInRegistry:  imagePresentOnRemote,
		ImageRuntime:     remoteRuntime(),
		// Outside a project the image ships alone: there are no DAGs here.
		ImageAlone:   m == nil,
		Description:  description,
		Interactive:  interactive,
		BuildSecrets: util.ResolveProjectBuildSecrets(deploySecrets, buildSecretSpecs(m)),
		CheckRuntime: func(runtime, airflowPin string) ([]runtimeversions.Finding, error) {
			return runtimecatalog.CheckRuntime(cmd.Context(), runtime, airflowPin)
		},
		Warn:   warnf,
		Engine: engine,
		Link: func(l scaffold.Link) error {
			if err := scaffold.SaveLink(config.WorkingPath, nil, l); err != nil {
				return err
			}
			fmt.Fprintf(errOut, "linked %s to apc deployment %s in %s\n", l.Name, l.Deployment, project.Marker)
			if after, err := manifest.Load(filepath.Join(config.WorkingPath, project.Marker)); err == nil {
				if kept := scaffold.NewlyDefault(m, after); kept != "" {
					fmt.Fprintf(errOut, "marked %s default = true: it was the project's only link, so its default, and stays so now there are two\n", kept)
				}
			}
			return nil
		},
		Platform:    manifest.KindAPC,
		Domain:      domain,
		OtherDomain: otherDomainThen("deploy again"),
		Announce: func(target manifestdeploy.Target) {
			announceDeploy(target, errOut, progress)
		},
		OnBuild: func() {
			fmt.Fprintln(progress, "Building your project image, this can take a few minutes...")
		},
	}, manifestdeploy.BeforeAsking(&apcManifestDeployer{client: houstonClient, in: cmd.InOrStdin(), errOut: errOut, progress: progress, warn: warnf}, recap.Flush))
	recap.Flush()
	if err != nil {
		if errors.Is(err, manifestdeploy.ErrAborted) {
			// Asked, and declined: the exit code says so.
			cmd.SilenceErrors = true
		}
		return err
	}
	return renderManifestDeploy(out, &res, warnings)
}

// refuseBuildSecret refuses --build-secret where nothing reads it, as astro
// deploy does on Astro: with --dags nothing is built, and a
// generated build mounts only the secret the runtime's install step reads.
// Gated on the flag, not the resolved value, which BUILD_SECRET_INPUT can
// fill without anyone asking.
func refuseBuildSecret(cmd *cobra.Command, m *manifest.Manifest) error {
	if !cmd.Flags().Changed("build-secret") {
		return nil
	}
	switch {
	case imageName != "":
		return errors.New("--build-secret has no effect with --image-name: the image is already built")
	case isDagOnlyDeploy:
		return errors.New("--build-secret has no effect with --dags: a dags-only deploy builds no image")
	case m.Astro.Dockerfile == "":
		return util.CheckGeneratedBuildSecrets(deploySecrets)
	}
	return nil
}

// otherDomainThen is the refusal of an apc link on another domain than the
// current context's: a command talks to one Houston, the context's, so the
// fix is to switch to that domain's context, or to log in to it first, and
// then to do again what was refused (again, such as "deploy again").
func otherDomainThen(again string) func(link, domain string) error {
	return func(link, domain string) error {
		if (&config.Context{Domain: domain}).ContextExists() {
			return fmt.Errorf("link %q is on %s, and the current context is another Astro Private Cloud: run astro context switch %s, then %s", link, domain, domain, again)
		}
		return fmt.Errorf("link %q is on %s, and this machine has no login there: run astro login %s, then %s", link, domain, domain, again)
	}
}

// renderManifestDeploy writes a finished project deploy: the one json object
// under json, and a summary line in text, after the progress the deploy
// printed as it went.
func renderManifestDeploy(out io.Writer, res *manifestdeploy.Result, warnings []string) error {
	obj := deployJSON{
		Deployment:     res.DeploymentID,
		Link:           res.LinkName,
		Workspace:      res.WorkspaceID,
		Type:           res.Type,
		Image:          res.ImageTag,
		DagsInImage:    res.DagsInImage,
		RuntimeVersion: res.RuntimeVersion,
		URL:            res.URL,
		Warnings:       warnings,
	}
	return cliout.Renderer{Format: deployOutput, Out: out}.Emit(obj, cliout.Text(func(b *bufio.Writer) {
		target := "Deployment " + res.DeploymentID
		if res.LinkName != "" {
			target = fmt.Sprintf("%s (Deployment %s)", res.LinkName, res.DeploymentID)
		}
		switch {
		case res.Type == deployTypeDags:
			fmt.Fprintf(b, "Deployed Dags to %s.\n", target)
		case res.DagsInImage:
			fmt.Fprintf(b, "Deployed image %s to %s, with the Dags inside the image.\n", res.ImageTag, target)
		case res.Type == deployTypeImageAndDags:
			fmt.Fprintf(b, "Deployed image %s and Dags to %s.\n", res.ImageTag, target)
		default:
			fmt.Fprintf(b, "Deployed image %s to %s.\n", res.ImageTag, target)
		}
		if res.URL != "" {
			fmt.Fprintf(b, "Deployment: %s\n", res.URL)
		}
	}))
}

// apcManifestDeployer wires internal/deploy's transport seam to Houston:
// the Deployments a workspace lists, the picker, and the image and DAG
// deploys of internal/platform/apc/deploy.
type apcManifestDeployer struct {
	client houston.ClientInterface
	// in and errOut carry the questions; progress takes the deploy's output.
	in       io.Reader
	errOut   io.Writer
	progress io.Writer
	// warn is the deploy's warning, for a DAG-only deploy's empty dags
	// directory: the request it takes carries none.
	warn func(string)
}

func (d *apcManifestDeployer) ConfirmTarget(choices []manifestdeploy.Choice, defaultLink string) (string, error) {
	return utils.ConfirmDeployTarget(choices, defaultLink, d.in, d.errOut)
}

// ListDeployments lists a workspace's Deployments, for the prompt to offer
// the ones the project does not link yet.
func (d *apcManifestDeployer) ListDeployments(workspaceID string) ([]manifestdeploy.Deployment, error) {
	deps, err := houston.Call(d.client.ListDeployments)(houston.ListDeploymentsRequest{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]manifestdeploy.Deployment, 0, len(deps))
	for i := range deps {
		out = append(out, manifestdeploy.Deployment{ID: deps[i].ID, Name: deps[i].Label, WorkspaceID: firstNonEmpty(deps[i].Workspace.ID, workspaceID)})
	}
	return out, nil
}

// errInvalidDeploySelection is the picker's answer to a choice that is not a
// row.
var errInvalidDeploySelection = errors.New("invalid Deployment selected")

// ResolveUnlinked asks which of a workspace's Deployments to ship to, for a
// project that links none.
func (d *apcManifestDeployer) ResolveUnlinked(workspaceID string) (manifestdeploy.Deployment, error) {
	list := picker.List{
		Title:   "Deploy to which Deployment?",
		Header:  []string{"DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID"},
		Ask:     []input.Option{input.About("the deployment to ship to"), input.AnsweredBy("--deployment")},
		Invalid: errInvalidDeploySelection,
	}
	if err := list.MayAsk(); err != nil {
		return manifestdeploy.Deployment{}, err
	}
	deps, err := houston.Call(d.client.ListDeployments)(houston.ListDeploymentsRequest{WorkspaceID: workspaceID})
	if err != nil {
		return manifestdeploy.Deployment{}, err
	}
	if len(deps) == 0 {
		return manifestdeploy.Deployment{}, fmt.Errorf("no Deployments in workspace %s to deploy to", workspaceID)
	}
	for i := range deps {
		list.AddRow(false, deps[i].Label, deps[i].ReleaseName, deps[i].ID)
	}
	i, err := list.Pick(d.errOut, d.in)
	if err != nil {
		return manifestdeploy.Deployment{}, err
	}
	return manifestdeploy.Deployment{ID: deps[i].ID, Name: deps[i].Label, WorkspaceID: firstNonEmpty(deps[i].Workspace.ID, workspaceID)}, nil
}

// DeployDags uploads the project's DAGs to a Deployment that takes DAG-only
// deploys, refusing one that does not. A dags directory with no DAGs in it
// is warned about rather than asked about, as on Astro: the upload goes
// ahead.
func (d *apcManifestDeployer) DeployDags(in *manifestdeploy.DagDeploy) (manifestdeploy.DagResult, error) {
	opts := deploy.Options{Progress: d.progress, WarnEmptyDags: d.warn}
	if _, err := DagsOnlyDeploy(d.client, in.WorkspaceID, in.DeploymentID, in.ProjectDir, nil, true, in.Description, opts); err != nil {
		return manifestdeploy.DagResult{}, err
	}
	return manifestdeploy.DagResult{WorkspaceID: in.WorkspaceID}, nil
}

// DeployImage builds the project's image, or takes a prebuilt one, and
// ships it.
func (d *apcManifestDeployer) DeployImage(in *manifestdeploy.ImageDeploy) (manifestdeploy.ImageResult, error) {
	res, err := DeployManifestImage(d.client, &deploy.ManifestImage{
		DeploymentID:   in.DeploymentID,
		WorkspaceID:    in.WorkspaceID,
		ImageName:      in.ImageName,
		InRegistry:     in.ImageInRegistry,
		RuntimeVersion: in.ImageRuntime,
		ImageAlone:     in.ImageAlone,
		Build:          in.Build,
		BuildSecrets:   in.BuildSecrets,
		IncludeDags:    in.IncludeDags,
		OnBuild:        in.OnBuild,
		Warn:           in.Warn,
		Survey:         in.Survey,
		Engine:         in.Engine,
		Description:    in.Description,
		Progress:       d.progress,
		Catalog:        runtimecatalog.Options(0),
		ResolveInstall: imageinstall.Fill,
	})
	if err != nil {
		return manifestdeploy.ImageResult{}, err
	}
	return manifestdeploy.ImageResult{
		WorkspaceID:      res.WorkspaceID,
		RuntimeVersion:   res.RuntimeVersion,
		ImageTag:         res.Image,
		DagsInImage:      res.DagsInImage,
		URL:              res.URL,
		DagDeployEnabled: res.DagsUploaded,
		DagsNotShipped:   res.DagsNotShipped,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// remoteRuntime is --runtime-version, which only --remote reads: an image on
// this machine, or one built, carries its runtime in its label.
func remoteRuntime() string {
	if !imagePresentOnRemote {
		return ""
	}
	return runtimeVersionForImageName
}

// buildSecretSpecs are the build secrets the manifest declares, none when
// there is no manifest: an --image-name deploy outside any project.
func buildSecretSpecs(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	return m.Astro.BuildSecretSpecs()
}

// announceDeploy says, in text, where a deploy is going once that is settled
// (on stderr, as every resolving command does), and that a prebuilt image
// is what it ships (with its progress).
func announceDeploy(target manifestdeploy.Target, errOut, progress io.Writer) {
	if deployOutput != cliout.FormatText {
		return
	}
	if target.LinkName == "" {
		fmt.Fprintf(errOut, "→ %s\n", target.DeploymentID)
	} else {
		fmt.Fprintf(errOut, "→ %s (apc deployment %s)\n", target.LinkName, target.DeploymentID)
	}
	if !isDagOnlyDeploy && imageName != "" {
		fmt.Fprintf(progress, "Deploying prebuilt image %s...\n", imageName)
	}
}
