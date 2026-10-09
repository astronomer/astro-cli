package astro

import (
	"bufio"
	"context"
	goerrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	dags               bool
	waitForDeploy      bool
	waitTime           time.Duration
	image              bool
	imageName          string
	deployDescription  string
	noDagsBaseDir      bool
	nonDags            bool
	nonDagsMountPath   string
	nonDagsBundleType  string
	nonDagsBundlePath  string
	manifestDeployment string
	manifestWorkspace  string
	deployOutput       string
	deployExample      = `  # Deploy this project, picking the Deployment from a list
  astro deploy

  # Deploy to a given Deployment
  astro deploy <DEPLOYMENT_ID>

  # Deploy only the DAGs
  astro deploy <DEPLOYMENT_ID> --dags

  # Deploy an image built on this machine, from any directory
  astro deploy <DEPLOYMENT_ID> --image-name <IMAGE_NAME>`

	buildSecrets = []string{}
)

const (
	deployWaitTime = 300 * time.Second

	imageNameFlag = "image-name"
	nonDagsFlag   = "non-dags"
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy [DEPLOYMENT_ID]",
		Short:   "Ship this project's code to a Deployment",
		Long:    "Deploy your project to a Deployment on Astro. Run it in a project with a pyproject.toml ([tool.astro]): it builds the project into a Docker image and pushes that image, with the project's DAGs, to Astronomer. A project in the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml) is converted with astro init, or deployed with Astro CLI 1.x. With --image-name it deploys an image built on this machine, from any directory. In Deployments with Remote Execution enabled, this only updates the Orchestration Plane components (the API Server and Scheduler). For all other components, use astro remote deploy instead. It does not include any metadata associated with your local Airflow environment.",
		Args:    cobra.MaximumNArgs(1),
		RunE:    deploy,
		Example: deployExample,
	}
	// --force answered 1.x's checks (uncommitted changes, an empty dags
	// directory), which v2's deploy does not make. It stays, accepted and
	// read by nothing, because astronomer/deploy-action passes it on every
	// deploy (TestDeployManifestDeployActionInvocations); hidden, as there is
	// nothing for anyone to choose.
	cmd.Flags().BoolP("force", "f", false, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("force") //nolint:errcheck // the flag is defined just above
	// --prompt asked 1.x to offer the Deployment list even with one named. A
	// deploy here asks whenever none is named, so it is accepted, hidden and
	// read by nothing, as a pyproject.toml project's deploy always took it.
	cmd.Flags().BoolP("prompt", "p", false, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working")
	cmd.Flags().MarkHidden("prompt") //nolint:errcheck // the flag is defined just above
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().StringVarP(&imageName, imageNameFlag, "i", "", "Name of a prebuilt local image to deploy instead of building one. Outside a pyproject.toml project it deploys the image alone")
	cmd.Flags().BoolVarP(&dags, "dags", "d", false, "Push only Dags to your Astro Deployment")
	cmd.Flags().BoolVar(&noDagsBaseDir, "no-dags-base-dir", false, "Exclude the dags directory prefix from the bundle. Use for Airflow 3.x deployments where sys.path includes the bundle root")
	cmd.Flags().BoolVarP(&image, "image", "", false, "Push only an image to your Astro Deployment; with Dag Deploy enabled, its Dags are not affected")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTime, "wait-time", deployWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	// No -d shorthand: on this command -d has meant --dags since v1, and moving
	// it would turn `astro deploy -d` from a DAG-only deploy into a target
	// selector. -d/--deployment is the spelling everywhere the letter is free.
	cmd.Flags().StringVar(&manifestDeployment, "deployment", "", "Deployment to deploy to: a link name from the manifest, or a Deployment id")
	cmd.Flags().StringVar(&manifestWorkspace, "workspace", "", "Workspace for the deploy, overriding the context")
	cmd.Flags().StringVar(&deployOutput, "output", string(cliout.FormatText), "Output format: text or json")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Add a description for more context on this deploy")
	utils.AddBuildSecretFlag(cmd.Flags(), &buildSecrets)
	cmd.Flags().BoolVar(&nonDags, nonDagsFlag, false, "Deploy a non-Dag bundle from a separate directory, instead of your Astro project. Requires --non-dags-mount-path")
	cmd.Flags().StringVar(&nonDagsMountPath, "non-dags-mount-path", "", "Path to mount the non-Dag bundle in Airflow, for reference by Dags. Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundleType, "non-dags-bundle-type", "none", "Free-form label identifying the kind of non-Dag bundle (e.g. dbt). Any value is accepted. Defaults to \"none\". Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundlePath, "non-dags-local-path", "", "Path to the non-Dag bundle to deploy. Default current directory. Used with --non-dags")

	annotateDeployFlag(cmd, "image", deployGroupImage)
	annotateDeployFlag(cmd, imageNameFlag, deployGroupImage)
	annotateDeployFlag(cmd, "build-secret", deployGroupImage)
	annotateDeployFlag(cmd, "dags", deployGroupDAG)
	annotateDeployFlag(cmd, "no-dags-base-dir", deployGroupDAG)
	annotateDeployFlag(cmd, nonDagsFlag, deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-mount-path", deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-bundle-type", deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-local-path", deployGroupNonDAG)
	orderDeployFlagGroups(cmd)
	utils.MarkPreferredFlag(cmd.Flags(), "workspace", "workspace-id")
	applyPreferredFlagsIn(cmd)
	return cmd
}

// deploy ships a project with a pyproject.toml ([tool.astro]), the one kind
// of project v2 deploys (deployManifest), run from the project's root.
//
// --non-dags deploys a directory that must not be inside a project at all, so
// where it runs from says nothing about the project. It is checked first.
//
// Everywhere else the project decides, by utils.Locate's walk. In or below a
// project in the Astro CLI 1.x layout every mode is refused, naming the two
// ways forward: convert it with astro init, or deploy it with Astro CLI 1.x,
// which keeps deploying that layout. --image-name is refused there too: a
// prebuilt image deployed from a 1.x checkout would leave that project's
// DAGs stale with nothing said. Below a pyproject.toml project's root, every
// mode is told to run from the root.
//
// Outside any project, --image-name is the one deploy: the image alone, with
// no DAGs, as there is no project to take them from, and no git commit. A
// Deployment that takes DAG deploys keeps the DAGs it had, and the deploy
// says so.
func deploy(cmd *cobra.Command, args []string) error {
	if nonDags {
		return deployNonDagsBundle(cmd, args)
	}
	// One walk routes the deploy and words its refusal.
	where := utils.Locate(config.WorkingPath)
	if where.AtRoot(config.WorkingPath) {
		return deployManifest(cmd, args, true)
	}
	if _, err := cliout.ParseFormat(deployOutput); err != nil {
		return err
	}
	// The value, not whether the flag was given: an empty --image-name=
	// names no image, and would be a build.
	if imageName != "" && where.None() {
		return deployManifest(cmd, args, false)
	}
	// Not a usage mistake, so no usage block under the error.
	cmd.SilenceUsage = true
	return utils.NoDeployableProject(where, utils.Deploy1xRefusedAstro)
}

// refuseFlagCombinations stops a deploy given flags that cannot all apply,
// before anything is read or asked, the way 1.x's deploy did:
//
//   - --wait-time sets how long --wait waits, so it needs --wait;
//   - --no-dags-base-dir lays out the DAGs a deploy ships, so it needs a
//     deploy that ships some: not --image, and not an --image-name deploy
//     outside a project, which ships the image alone.
//
// inProject is whether the deploy runs at a pyproject.toml project's root.
func refuseFlagCombinations(cmd *cobra.Command, inProject bool) error {
	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return cliout.Usage(errors.New("cannot use --wait-time with --wait=false"))
	}
	if cmd.Flags().Changed("no-dags-base-dir") {
		switch {
		case image:
			return cliout.Usage(errors.New("cannot use --no-dags-base-dir with --image: an image-only deploy ships no DAGs"))
		case imageName != "" && !inProject:
			return cliout.Usage(errors.New("cannot use --no-dags-base-dir with --image-name outside a pyproject.toml project: the image ships alone, with no DAGs"))
		}
	}
	return nil
}

// nonDagsDeployJSON is what `astro deploy --non-dags --output json`
// publishes: the bundle deployed and where. Its keys are the ones
// `astro dbt deploy`'s result uses for the same facts.
type nonDagsDeployJSON struct {
	Deployment     string `json:"deployment"`
	DeploymentName string `json:"deployment_name"`
	Workspace      string `json:"workspace"`
	DeployID       string `json:"deploy_id"`
	// BundleType is --non-dags-bundle-type, BundlePath the directory bundled.
	BundleType    string `json:"bundle_type"`
	BundlePath    string `json:"bundle_path"`
	MountPath     string `json:"mount_path"`
	BundleVersion string `json:"bundle_version"`
	// Waited is true when the run waited for the Deployment to become
	// healthy (--wait), and WaitError says why that wait failed.
	Waited    bool           `json:"waited"`
	WaitError string         `json:"wait_error,omitempty"`
	Git       *deployGitJSON `json:"git,omitempty"`
}

func newNonDagsDeployJSON(res *astrodeploy.BundleDeploy, bundleType, bundlePath string, waited bool, waitErr error) nonDagsDeployJSON {
	// The facts a dbt bundle deploy publishes, less its dbt project.
	d := newDbtDeployJSON(res, "", bundlePath, waited, waitErr)
	return nonDagsDeployJSON{
		Deployment: d.Deployment, DeploymentName: d.DeploymentName, Workspace: d.Workspace, DeployID: d.DeployID,
		BundleType: bundleType, BundlePath: d.ProjectPath, MountPath: d.MountPath, BundleVersion: d.BundleVersion,
		Waited: d.Waited, WaitError: d.WaitError, Git: d.Git,
	}
}

func deployNonDagsBundle(cmd *cobra.Command, args []string) error {
	// Read first, so a bad --output is the usage error reported.
	format, err := cliout.ParseFormat(deployOutput)
	if err != nil {
		return err
	}
	for _, f := range []string{"dags", "image", imageNameFlag, "build-secret", "no-dags-base-dir"} {
		if cmd.Flags().Changed(f) {
			return cliout.Usage(fmt.Errorf("cannot use --%s with --non-dags; --non-dags performs a non-Dag bundle deploy", f))
		}
	}

	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return cliout.Usage(errors.New("cannot use --wait-time with --wait=false"))
	}

	if nonDagsMountPath == "" {
		return cliout.Usage(errors.New("--non-dags-mount-path is required with --non-dags"))
	}

	if nonDagsBundlePath == "" {
		nonDagsBundlePath = config.WorkingPath
	}

	info, err := os.Stat(nonDagsBundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle path %s does not exist", nonDagsBundlePath)
		}
		return fmt.Errorf("failed to access bundle path %s: %w", nonDagsBundlePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path %s is not a directory", nonDagsBundlePath)
	}

	// The walk every deploy decides by: a pyproject.toml project or a 1.x
	// one at or above the bundle path.
	if !utils.Locate(nonDagsBundlePath).None() {
		return errors.New("bundle path is within an Astro project. Non-Dag bundles must be a separate directory")
	}

	targetID, target, login, err := nonDagsTarget(cmd.Context(), args, format)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true

	deployBundleInput := &astrodeploy.DeployBundleInput{
		BundlePath:    nonDagsBundlePath,
		MountPath:     nonDagsMountPath,
		DeploymentID:  targetID,
		Deployment:    target,
		BundleType:    nonDagsBundleType,
		Description:   deployDescription,
		AstroV1Client: login.client,
	}
	if !login.current {
		deployBundleInput.Login = &login.context
	}
	res, err := DeployBundle(deployBundleInput)
	if err != nil {
		return err
	}
	waitDone := func() error {
		if login.current {
			return waitForBundle(cmd.ErrOrStderr(), res.DeploymentID, waitTime, login.client)
		}
		return astrodeploy.WaitForBundleIn(cmd.ErrOrStderr(), &login.context, res.DeploymentID, waitTime, login.client)
	}
	return publishThenWaitWith(cmd, format, waitForDeploy, waitDone, func(waitErr error) error {
		if format == cliout.FormatJSON {
			return cliout.Renderer{Format: format, Out: cmd.OutOrStdout()}.Emit(newNonDagsDeployJSON(&res, nonDagsBundleType, nonDagsBundlePath, waitForDeploy, waitErr), nil)
		}
		return renderBundleUploaded(res.BundleVersion)(cmd.OutOrStdout())
	})
}

// nonDagsTarget is the Deployment a non-Dag bundle deploys to, and the login
// it deploys under, named as a project deploy names them: the argument or
// --deployment, which must agree. In a pyproject.toml project here that
// loads, the deploy runs under the login for the project's Astro host, as its
// deploy does: a link name is that link's Deployment, and any other name a
// Deployment id. Anywhere else, or when the project does not load, the name
// is an id under the current context: an id needs no manifest.
// With no name, the workspace's Deployments are offered: --workspace, else
// --workspace-id, else the context's.
func nonDagsTarget(ctx context.Context, args []string, format cliout.Format) (string, *astrov1.Deployment, deployLogin, error) {
	current, _ := loginForDeploy(ctx, "") //nolint:errcheck // with no domain it is the current context, and never fails
	linkName := ""
	if len(args) > 0 {
		linkName = args[0]
	}
	ws := manifestWorkspace
	if ws == "" {
		ws = workspaceID
	}
	// Agreement first, before anything is read.
	if _, _, err := manifestdeploy.ResolveNamed(nil, linkName, manifestDeployment, ws, ""); err != nil {
		return "", nil, current, cliout.Usage(err)
	}
	if name := firstNonEmptyString(linkName, manifestDeployment); name != "" {
		m := manifestHere()
		if m == nil {
			// No project here, or one that does not load: an id, under the
			// current context.
			return name, nil, current, nil
		}
		// In a project, link and id alike go to the project's Astro host,
		// as its deploy does.
		login, err := loginForDeploy(ctx, m.Astro.LoginDomain())
		if err != nil {
			return "", nil, current, err
		}
		if !hasLink(m, name) {
			return name, nil, login, nil
		}
		named, _, err := manifestdeploy.ResolveNamed(m, name, "", ws, "")
		if err != nil {
			return "", nil, current, cliout.Usage(err)
		}
		return named.DeploymentID, nil, login, nil
	}
	if ws == "" {
		var err error
		ws, err = coalesceWorkspace()
		if err != nil {
			return "", nil, current, errors.Wrap(err, "failed to find a valid workspace")
		}
	}
	id, read, err := resolveBundleDeployment(nil, ws, "", noCreateUnderJSON(format, "to deploy to"))
	return id, read, current, err
}

// manifestHere is the pyproject.toml project's manifest in the working
// directory, or nil when there is none or it does not load.
func manifestHere() *manifest.Manifest {
	if !utils.IsManifestRoot(config.WorkingPath) {
		return nil
	}
	m, err := manifest.Load(filepath.Join(config.WorkingPath, project.Marker))
	if err != nil {
		return nil
	}
	return m
}

func hasLink(m *manifest.Manifest, name string) bool {
	_, ok := m.Astro.Deployments[name]
	return ok
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// deployManifest runs the manifest deploy path: load the manifest, gather flags and
// context, resolve the deployment, and run the deploy — dags-only, image-only,
// or both — then render the result. The manifest deploy's logic lives in internal/deploy; this
// is the cmd shim that parses, wires the transport, and prints.
//
// inProject is false for an --image-name deploy outside a pyproject.toml
// project: there is no manifest, so the target is a Deployment id (the
// argument or --deployment) or the workspace's pick, the login is the current
// context's, and the image ships alone, with no DAGs.
func deployManifest(cmd *cobra.Command, args []string, inProject bool) error {
	// The format is read before the flag refusals, so a bad --output is the
	// usage error reported rather than whichever refusal came first. Every
	// failure below, refusals included, reaches a json-mode caller as the one
	// {"error","code","kind"} object on stdout: the root reports it
	// (cliout.Execute). A script reading stdout to find out what went wrong is
	// who these refusals exist for.
	format, err := cliout.ParseFormat(deployOutput)
	if err != nil {
		return err
	}
	if err := refuseFlagCombinations(cmd, inProject); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	out := cmd.OutOrStdout()

	var m *manifest.Manifest
	if inProject {
		m, err = manifest.Load(filepath.Join(config.WorkingPath, "pyproject.toml"))
		if err != nil {
			return manifestDeployErr(cmd, err)
		}
	}

	// --build-secret is refused HERE, not in internal/deploy, and gated on the
	// FLAG rather than on the resolved value. Three regressions came from getting
	// this wrong when the blanket refusal was removed:
	//
	//   - ResolveBuildSecrets reads BUILD_SECRET_INPUT whether or not the flag was
	//     given, so a refusal keyed on the resolved slice broke every ordinary
	//     manifest deploy on any runner that exports that variable — no flag, no
	//     Dockerfile, and a hard error telling the user to declare one they never
	//     wanted. cmd.Flags().Changed is what the old refusal used.
	//   - --dags never reaches runImage, so a refusal there let
	//     `--dags --build-secret` succeed with the flag silently dropped.
	//   - --image-name returns before any build, so the same silent drop applied
	//     whenever the project happened to declare a Dockerfile.
	//
	// One place, before anything is resolved or prompted for.
	if err := refuseBuildSecret(cmd, m); err != nil {
		return manifestDeployErr(cmd, err)
	}

	linkName := ""
	if len(args) > 0 {
		linkName = args[0]
	}

	// Without a project there is no host of its own, and no build to
	// declare secrets for.
	domain, secretSpecs := "", []string(nil)
	if m != nil {
		domain, secretSpecs = m.Astro.LoginDomain(), m.Astro.BuildSecretSpecs()
	}
	login, err := loginForDeploy(cmd.Context(), domain)
	if err != nil {
		return manifestDeployErr(cmd, err)
	}

	// --workspace wins over the legacy --workspace-id when both are set.
	overrideWorkspace := manifestWorkspace
	if overrideWorkspace == "" {
		overrideWorkspace = workspaceID
	}

	// A prompt has to be answered by someone, and json output has to stay a
	// stream a program can parse — a question on stderr with the run blocked on
	// stdin is not that. So json mode is non-interactive whatever stdin is, and
	// must name its target like any other script.
	interactive := format == cliout.FormatText && stdinIsTerminal()

	// Only a run that is going to ask has any use for the ambient layers, and
	// reading the pin is not free — it creates the project's state directory.
	willPrompt := interactive && inProject && linkName == "" && manifestDeployment == ""
	preselect, preselectFrom := deployPreselect(config.WorkingPath, willPrompt)

	errOut := cmd.ErrOrStderr()
	res, err := manifestdeploy.Run(manifestdeploy.Request{
		ProjectDir:       config.WorkingPath,
		Manifest:         m,
		LinkName:         linkName,
		Deployment:       manifestDeployment,
		Preselect:        preselect,
		PreselectFrom:    preselectFrom,
		WorkspaceID:      overrideWorkspace,
		ContextWorkspace: login.context.Workspace,
		DagsOnly:         dags,
		// Outside a project the image ships alone: there are no DAGs here.
		Image:         image,
		ImageAlone:    !inProject,
		ImageName:     imageName,
		Description:   deployDescription,
		Wait:          waitForDeploy,
		WaitTime:      waitTime,
		NoDagsBaseDir: noDagsBaseDir,
		Interactive:   interactive,
		// Astro CLI 1.x's resolution, so a CI job setting BUILD_SECRET_INPUT
		// keeps working across the version boundary rather than silently
		// losing its secrets on the day the project converts. With neither the
		// flag nor the variable, the manifest's build-secrets apply.
		BuildSecrets: util.ResolveProjectBuildSecrets(buildSecrets, secretSpecs),
		// The runtime build is checked against the catalog where the image is
		// about to be built from it. Its warnings go to stderr, so a json run's
		// stdout stays the one result object.
		CheckRuntime: func(runtime, airflowPin string) ([]runtimeversions.Finding, error) {
			return runtimecatalog.CheckRuntime(cmd.Context(), runtime, airflowPin)
		},
		Warn: func(msg string) { fmt.Fprintf(errOut, "warning: %s\n", msg) },
		// Two lines. The first is the → line every resolving command prints,
		// once the target is settled, so a deploy says what it is about to act
		// on the way `astro af dags list` does. The second says an image build
		// can run for minutes with no transport output yet; the transport
		// itself stays silent (the core's layer rules). It prints only once the
		// deployment has cleared the deploy, so a deploy refused before the
		// build claims no build.
		//
		// They go to stderr and stdout respectively — the announce line is
		// context, the progress line is this command's own output — and json
		// mode drops both, so the single result object is all it adds.
		Announce: func(target manifestdeploy.Target) {
			if format != cliout.FormatText {
				return
			}
			announceDeployTarget(errOut, target)
			if !dags && imageName != "" {
				fmt.Fprintf(out, "Deploying prebuilt image %s...\n", imageName)
			}
		},
		OnBuild: func() {
			if format == cliout.FormatText {
				fmt.Fprintln(out, "Building your project image, this can take a few minutes...")
			}
		},
	}, newManifestDeployer(&login, cmd.InOrStdin(), errOut))
	if err != nil {
		return manifestDeployErr(cmd, err)
	}

	if res.Git.Uncommitted {
		fmt.Fprintln(errOut, "note: the project has uncommitted changes, so this deploy records no git commit")
	}
	var warnings []string
	if !inProject {
		// The image shipped alone. A Deployment that takes DAG deploys
		// keeps the DAGs it had; one that does not runs whatever DAGs the
		// image carries, which may be none.
		w := fmt.Sprintf(warningDagsInImage, res.DeploymentID, imageName)
		if res.DagDeployEnabled {
			w = fmt.Sprintf(warningDagsNotUpdated, res.DeploymentID, res.DeploymentID)
		}
		warnings = append(warnings, w)
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	return renderManifestDeploy(out, format, &res, warnings)
}

// announceDeployTarget prints the one line every resolving command puts on
// stderr before it acts, so a deploy's target is never invisible either. It
// matches cmd/local's announceInstance: the name, then what the name does not
// already say. A deployment named by id has nothing to add.
func announceDeployTarget(w io.Writer, target manifestdeploy.Target) {
	if target.LinkName == "" {
		fmt.Fprintf(w, "→ %s\n", target.DeploymentID)
		return
	}
	fmt.Fprintf(w, "→ %s (astro deployment %s)\n", target.LinkName, target.DeploymentID)
}

// stdinIsTerminal reports whether this run can be asked which deployment to
// ship to. It is a var rather than the call itself so a test can drive the
// prompt without a pty.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// deployPreselect is what the ambient layers point at, for the prompt's cursor
// and nothing else: ASTRO_DEPLOYMENT, then this project's `astro use` pin, in
// the order the query commands rank them. It reports which one answered, so the
// prompt can label the highlight with the thing that actually caused it.
//
// A pin that cannot be read is not worth failing a deploy over — the worst it
// costs is a cursor on the first entry — so the error is dropped. And a run with
// no prompt coming never reads the pin at all: userstate.Load creates the
// project's state directory on the way past, and a `astro deploy prod` in CI
// should not leave a cache directory behind for a question nobody asked.
func deployPreselect(projectDir string, willPrompt bool) (name, from string) {
	if name := os.Getenv(instances.EnvVar); name != "" {
		return name, instances.EnvVar
	}
	if !willPrompt {
		return "", ""
	}
	state, err := userstate.Load(projectDir)
	if err != nil {
		return "", ""
	}
	if state.Instance == "" {
		return "", ""
	}
	return state.Instance, manifestdeploy.PinnedBy
}

// deployJSON is the single object `astro deploy --output json` emits when a manifest
// deploy finishes. Fields that do not apply to a deploy kind are omitted: a
// dags-only deploy carries no image_tag, an image-only deploy no
// dag_bundle_version.
type deployJSON struct {
	Deployment string `json:"deployment"`
	// Link is the manifest link the deploy resolved to, omitted when the target
	// was named by id. It is what the person typed and what their teammates
	// call it; the id alone makes a consumer look it up again.
	Link             string         `json:"link,omitempty"`
	Workspace        string         `json:"workspace"`
	Type             string         `json:"type"`
	ImageTag         string         `json:"image_tag,omitempty"`
	DagBundleVersion string         `json:"dag_bundle_version,omitempty"`
	RuntimeVersion   string         `json:"runtime_version,omitempty"`
	URL              string         `json:"url,omitempty"`
	Git              *deployGitJSON `json:"git,omitempty"`
	// Warnings are what the deploy warned about without failing, as text
	// prints them on stderr (less the "warning: " prefix): an image shipped
	// alone to a Deployment that takes DAG deploys, whose DAGs it left as
	// they were.
	Warnings []string `json:"warnings,omitempty"`
}

// warningDagsNotUpdated is the warning for an --image-name deploy outside a
// project to a Deployment that takes DAG deploys: the image changed, its DAGs
// did not.
const warningDagsNotUpdated = "this deploy shipped the image alone, so Deployment %s keeps the DAGs it had. " +
	"To update them, run astro deploy %s --dags from the project directory"

// warningDagsInImage is the same deploy to a Deployment that takes no DAG
// deploys: it runs the DAGs inside the image, which no project here put
// there. APC's deploy warns the same (warningImageNameDagsInImage).
const warningDagsInImage = "Deployment %s runs the DAGs inside the image %s; no dags folder was deployed. " +
	"An image astro package built without a dockerfile declared under [tool.astro] contains none"

type deployGitJSON struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch,omitempty"`
	CommitURL string `json:"commit_url,omitempty"`
}

// renderManifestDeploy writes a finished manifest deploy: one JSON object in json mode, a
// plain summary in text mode. Both render the same Result, so the two modes
// never drift.
//
// The object is a result, not the end of a stream: a manifest deploy emits no
// stream record before it (build progress and warnings go to stderr, and json
// mode drops the announce and progress lines), so it is laid out the way
// every result is, pretty on a terminal and one line when piped.
func renderManifestDeploy(w io.Writer, format cliout.Format, res *manifestdeploy.Result, warnings []string) error {
	obj := deployJSON{
		Warnings:         warnings,
		Deployment:       res.DeploymentID,
		Link:             res.LinkName,
		Workspace:        res.WorkspaceID,
		Type:             res.Type,
		ImageTag:         res.ImageTag,
		DagBundleVersion: res.DagTarballVersion,
		RuntimeVersion:   res.RuntimeVersion,
		URL:              res.URL,
	}
	if c := res.Git.Commit; c != nil {
		obj.Git = &deployGitJSON{CommitSHA: c.SHA, Branch: c.Branch, CommitURL: c.URL}
	}
	return cliout.Renderer{Format: format, Out: w}.Emit(obj, cliout.Text(func(b *bufio.Writer) {
		target := deployTargetName(res)
		switch res.Type {
		case "dag-only":
			fmt.Fprintf(b, "Deployed DAGs (version %s) to %s.\n", res.DagTarballVersion, target)
		case "image-only":
			fmt.Fprintf(b, "Deployed image (tag %s) to %s.\n", res.ImageTag, target)
		default: // image-and-dag
			fmt.Fprintf(b, "Deployed image (tag %s) and DAGs (version %s) to %s.\n", res.ImageTag, res.DagTarballVersion, target)
		}
		if res.URL != "" {
			fmt.Fprintf(b, "Deployment: %s\n", res.URL)
		}
	}))
}

// deployTargetName is what the summary line calls where the code went: the link
// name the user chose, with the id in tow, because the id alone is the one thing
// nobody recognizes at a glance.
func deployTargetName(res *manifestdeploy.Result) string {
	if res.LinkName == "" {
		return "deployment " + res.DeploymentID
	}
	return fmt.Sprintf("%s (deployment %s)", res.LinkName, res.DeploymentID)
}

// manifestDeployErr returns a manifest deploy failure for the root to report: the
// {"error","code","kind"} object on stdout in json mode, cobra's "Error:" in
// text mode (cliout.Execute, as for every command).
func manifestDeployErr(cmd *cobra.Command, err error) error {
	if goerrors.Is(err, manifestdeploy.ErrAborted) {
		// The user was asked and said no. Reading their own answer back at them
		// as "Error: no deployment selected" adds nothing; the exit code carries
		// the whole message. Only an interactive run can reach here, which is
		// text mode, so this never eats the object json mode promises.
		cmd.SilenceErrors = true
	}
	return err
}

// deployLogin is the Astro login a manifest deploy runs under, and the client bound
// to its host and token.
type deployLogin struct {
	context config.Context
	client  astrov1.APIClient
	// current reports the login is the current context's, which the
	// workspace-level deployment picker assumes.
	current bool
}

// loginForDeploy picks the login a manifest deploy runs under from domain, the
// project's Astro host (manifest.Astro.LoginDomain): the login stored for that
// host, even while the CLI is switched to another, the same one `astro af`
// reads an astro link with. Every Deployment the deploy reaches, a link's or a
// bare id's, is looked up on that host. A project that names no host, or names
// the current one, deploys under the current context.
func loginForDeploy(ctx context.Context, domain string) (deployLogin, error) {
	current, _ := config.GetCurrentContext() //nolint:errcheck // with no context, the transport reports it as it always has
	if domain == "" || manifest.NormalizeDomain(current.Domain) == domain {
		return deployLogin{context: current, client: astroV1Client, current: true}, nil
	}
	token, err := astrosession.BearerFor(ctx, domain)
	if err != nil {
		return deployLogin{}, err
	}
	org, err := instancelocate.Organization(domain)
	if err != nil {
		return deployLogin{}, err
	}
	login, err := (&config.Context{Domain: domain}).GetContext()
	if err != nil {
		return deployLogin{}, err
	}
	// The token as BearerFor gave it, a stored one with its scheme or
	// ASTRO_API_TOKEN as set: read by the one rule, so the scheme goes out
	// once whichever it was.
	token = "Bearer " + astrosession.Credential(token)
	login.Token, login.Organization = token, org
	return deployLogin{
		context: login,
		client:  astrov1.NewV1ClientForLogin(httputil.NewHTTPClient(), token, login.GetPublicRESTAPIURL("v1")),
	}, nil
}

// newManifestDeployer builds the transport the manifest deploy path drives. It is a var so a
// test can swap in a fake and exercise the whole cmd path — flag parsing,
// selection, and rendering — with no real daemon, registry, or API.
var newManifestDeployer = func(login *deployLogin, in io.Reader, errOut io.Writer) manifestdeploy.Deployer {
	return manifestDeployer{login: login, in: in, errOut: errOut}
}

// manifestDeployer wires internal/deploy's transport seam to the Astro deploy
// transport (internal/platform/astro/deploy) and the deployment selection flow.
type manifestDeployer struct {
	login *deployLogin
	// in and errOut carry the deploy prompt. It asks on stderr and reads stdin,
	// so stdout stays the deploy's own output.
	in     io.Reader
	errOut io.Writer
}

// ConfirmTarget asks which deployment to ship to. Every interactive deploy that
// did not name its target comes through here — a pin, ASTRO_DEPLOYMENT, or a
// `default = true` marker moves the cursor and never skips the question
// (docs/instances.md, "Deploy always asks").
//
// The highlight is labeled with what put it there, not with one word for all
// three: a cursor sitting on an entry because a variable is exported in this
// shell is a different fact from one sitting there because the committed
// manifest says so, and only the reader can tell which they meant.
//
// It takes a name or a number, names first: a link may legally be called "2",
// and what the user typed is what they meant. Enter takes the highlighted entry
// when there is one. With nothing highlighted there is no default on Enter: the
// safe answer to "where should I ship this code" is never one the CLI picked by
// itself. It asks on stderr, so stdout stays the deploy's own output.
func (d manifestDeployer) ConfirmTarget(choices []manifestdeploy.Choice, preselect manifestdeploy.Preselect) (string, error) {
	header := []string{"NAME", "WHERE"}
	if preselect.Name != "" {
		header = append(header, "PRESELECTED BY")
	}
	l := picker.List{
		Title:  "Deploy to which deployment?",
		Header: header,
		Ask:    []input.Option{input.About("the deployment to ship to"), input.AnsweredBy("--deployment")},
		// Asked picker.DefaultAttempts times, and still no pick. The picker has said "Not one
		// of the choices." after every answer, the third included, so that
		// line is the last word and this ends quietly: that is what the
		// sentinel is for. Input that ends instead fails with Ended.
		Invalid: manifestdeploy.ErrAborted,
		ByName:  true,
		Ended:   input.Required(errors.New("a deploy must name the deployment it ships to: `astro deploy <name>` or --deployment <name>")),
	}
	for i, choice := range choices {
		cells := []string{choice.Name, choice.Where}
		chosen := preselect.Name != "" && choice.Name == preselect.Name
		switch {
		case chosen:
			l.Default = i + 1
			cells = append(cells, preselect.From)
		case preselect.Name != "":
			cells = append(cells, "")
		}
		l.AddRow(chosen, cells...)
	}
	i, err := l.Pick(d.errOut, d.in)
	if err != nil {
		return "", err
	}
	return choices[i].Name, nil
}

// ResolveUnlinked runs the workspace-level pick/create flow and returns the
// chosen deployment id.
func (d manifestDeployer) ResolveUnlinked(workspaceID string) (string, error) {
	if !d.login.current {
		return "", fmt.Errorf("this project deploys to %[1]s, and the deployment picker lists only the current context's Deployments. Pass --deployment <id>, or run `astro context switch %[1]s`", d.login.context.Domain)
	}
	dep, err := deployment.GetDeployment(workspaceID, "", "", false, nil, d.login.client)
	if err != nil {
		return "", err
	}
	return dep.Id, nil
}

// DeployDags ships the project's dags/ through the dags-only transport.
func (d manifestDeployer) DeployDags(in *manifestdeploy.DagDeploy) (manifestdeploy.DagResult, error) {
	res, err := astrodeploy.DeployManifestDags(astrodeploy.ManifestDagDeployInput{
		Login:         &d.login.context,
		ProjectDir:    in.ProjectDir,
		DeploymentID:  in.DeploymentID,
		Description:   in.Description,
		NoDagsBaseDir: in.NoDagsBaseDir,
		Wait:          in.Wait,
		WaitTime:      in.WaitTime,
		Progress:      d.errOut,
	}, d.login.client)
	if err != nil {
		return manifestdeploy.DagResult{}, err
	}
	return manifestdeploy.DagResult{
		WorkspaceID:       res.WorkspaceID,
		RuntimeVersion:    res.RuntimeVersion,
		DagTarballVersion: res.DagTarballVersion,
		URL:               res.URL,
		Git:               toManifestDeployGit(res.Git),
	}, nil
}

func toManifestDeployGit(g astrodeploy.ManifestDeployGit) manifestdeploy.Git {
	out := manifestdeploy.Git{Uncommitted: g.Uncommitted}
	if c := g.Commit; c != nil {
		out.Commit = &manifestdeploy.Commit{SHA: c.CommitSha}
		if c.Branch != nil {
			out.Commit.Branch = *c.Branch
		}
		if c.CommitUrl != nil {
			out.Commit.URL = *c.CommitUrl
		}
	}
	return out
}

// DeployImage builds or adopts the project image and ships it through the
// cloud/deploy transport.
func (d manifestDeployer) DeployImage(in *manifestdeploy.ImageDeploy) (manifestdeploy.ImageResult, error) {
	res, err := astrodeploy.DeployManifestImage(astrodeploy.ManifestImageDeployInput{
		Login:         &d.login.context,
		Build:         in.Build,
		Catalog:       func() *runtimeversions.Catalog { return runtimecatalog.Catalog(context.Background()) },
		DeploymentID:  in.DeploymentID,
		BuildSecrets:  in.BuildSecrets,
		ImageName:     in.ImageName,
		OnBuild:       in.OnBuild,
		IncludeDags:   in.IncludeDags,
		Description:   in.Description,
		NoDagsBaseDir: in.NoDagsBaseDir,
		Wait:          in.Wait,
		WaitTime:      in.WaitTime,
		Progress:      d.errOut,
	}, d.login.client)
	if err != nil {
		return manifestdeploy.ImageResult{}, err
	}
	return manifestdeploy.ImageResult{
		WorkspaceID:       res.WorkspaceID,
		RuntimeVersion:    res.RuntimeVersion,
		ImageTag:          res.ImageTag,
		DagTarballVersion: res.DagTarballVersion,
		URL:               res.URL,
		Git:               toManifestDeployGit(res.Git),
		DagDeployEnabled:  res.DagDeployEnabled,
	}, nil
}

// isWithinManifestProject reports whether path sits at or inside a project
// with a pyproject.toml ([tool.astro]).
//
// config.IsWithinProjectDir only knows the 1.x marker, .astro/config.yaml, so it
// answers false at every level of a project with a pyproject.toml. That left both containment
// refusals in this package unreachable for exactly the projects that have a manifest:
// a dbt project or a non-DAG bundle nested inside one shipped where the check
// meant to stop it.
func isWithinManifestProject(path string) bool {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	for dir := abs; ; {
		if project.HasManifest(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// refuseBuildSecret is deployManifest's --build-secret check (see the
// comment where it is called): the flag where nothing is built, or a secret
// a generated build cannot mount. m is nil only for an --image-name deploy
// outside a project, which the second case stops first.
func refuseBuildSecret(cmd *cobra.Command, m *manifest.Manifest) error {
	if !cmd.Flags().Changed("build-secret") {
		return nil
	}
	switch {
	case dags:
		return errors.New("--build-secret has no effect with --dags: a dags-only deploy builds no image")
	case imageName != "":
		return errors.New("--build-secret has no effect with --image-name: the image is already built")
	case m.Astro.Dockerfile == "":
		return util.CheckGeneratedBuildSecrets(buildSecrets)
	}
	return nil
}
