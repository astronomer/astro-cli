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
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	forceDeploy        bool
	forcePrompt        bool
	saveDeployConfig   bool
	pytest             bool
	parse              bool
	dags               bool
	waitForDeploy      bool
	waitTime           time.Duration
	image              bool
	dagsPath           string
	pytestFile         string
	envFile            string
	imageName          string
	deploymentName     string
	deployDescription  string
	noDagsBaseDir      bool
	dagBundleName      string
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
  astro deploy <DEPLOYMENT_ID> --dags`

	DeployImage      = astrodeploy.Deploy
	EnsureProjectDir = utils.EnsureProjectDir
	buildSecrets     = []string{}
	// hasUncommittedChanges is a variable so a test does not depend on the
	// state of the checkout it runs in.
	hasUncommittedChanges = git.HasUncommittedChanges

	errUncommittedChanges = errors.New("project directory has uncommitted changes: commit them, or use astro deploy [deployment-id] --force to deploy anyway")
)

const (
	deployWaitTime = 300 * time.Second

	imageNameFlag = "image-name"
	nonDagsFlag   = "non-dags"
	dagsPathFlag  = "dags-path"
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [DEPLOYMENT_ID]",
		Short: "Ship this project's code to a Deployment",
		Long:  "Deploy your project to a Deployment on Astro. This command bundles your project files into a Docker image and pushes that Docker image to Astronomer. In Deployments with Remote Execution enabled, this only updates the Orchestration Plane components (the API Server and Scheduler). For all other components, use astro remote deploy instead. It does not include any metadata associated with your local Airflow environment.",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed(imageNameFlag) || cmd.Flags().Changed(nonDagsFlag) {
				return nil
			}
			// A project with a pyproject.toml has no .astro/config.yaml, so the 1.x EnsureProjectDir
			// check would reject it. The manifest path loads and validates the manifest
			// itself, so skip the 1.x check and let deploy() route.
			if project.HasManifest(config.WorkingPath) {
				return nil
			}
			// A DAG-only deploy sourcing its DAGs from --dags-path does not read anything
			// else from the working directory, unless --pytest/--parse is also used, which
			// builds and runs a local image to test-parse the DAGs and so still needs the
			// project root. Check the bound values (not cmd.Flags().Changed) so --dags=false
			// or an explicit empty --dags-path cannot be misread as enabling the bypass.
			if dags && dagsPath != "" && !pytest && !parse {
				return nil
			}
			return EnsureProjectDir(cmd, args)
		},
		RunE:    deploy,
		Example: deployExample,
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy even if project contains errors or uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().BoolVar(&pytest, "pytest", false, "Deploy code to Astro only if the specified Pytests are passed")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables for Pytests")
	cmd.Flags().StringVarP(&pytestFile, "test", "t", "", "Location of Pytests or specific Pytest file. All Pytest files must be located in the tests directory")
	cmd.Flags().StringVarP(&imageName, imageNameFlag, "i", "", "Name of a custom image to deploy, or image name with custom tag when used with --client")
	cmd.Flags().BoolVarP(&dags, "dags", "d", false, "Push only Dags to your Astro Deployment")
	cmd.Flags().BoolVar(&noDagsBaseDir, "no-dags-base-dir", false, "Exclude the dags directory prefix from the bundle. Use for Airflow 3.x deployments where sys.path includes the bundle root")
	cmd.Flags().StringVar(&dagBundleName, "dag-bundle-name", "", "Deploy Dags to a named Dag bundle on the Deployment instead of the default bundle. Requires Airflow 3, and the bundle must already exist on the Deployment")
	cmd.Flags().BoolVarP(&image, "image", "", false, "Push only an image to your Astro Deployment; with Dag Deploy enabled, its Dags are not affected")
	cmd.Flags().StringVar(&dagsPath, dagsPathFlag, "", "If set deploy dags from this path instead of the dags from working directory")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to deploy to")
	cmd.Flags().BoolVar(&parse, "parse", false, "Succeed only if all Dags in your Astro project parse without errors")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTime, "wait-time", deployWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cmd.Flags().MarkHidden("dags-path") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	// No -d shorthand: on this command -d has meant --dags since v1, and moving
	// it would turn `astro deploy -d` from a DAG-only deploy into a target
	// selector. -d/--deployment is the spelling everywhere the letter is free.
	cmd.Flags().StringVar(&manifestDeployment, "deployment", "", "Deployment to deploy to: a link name from the manifest, or a Deployment id. In a project with a pyproject.toml ([tool.astro])")
	cmd.Flags().StringVar(&manifestWorkspace, "workspace", "", "Workspace for the deploy, overriding the context")
	cmd.Flags().StringVar(&deployOutput, "output", string(cliout.FormatText), "Output format in a project with a pyproject.toml ([tool.astro]): text or json")
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
	annotateDeployFlag(cmd, "dag-bundle-name", deployGroupDAG)
	annotateDeployFlag(cmd, dagsPathFlag, deployGroupDAG)
	annotateDeployFlag(cmd, "pytest", deployGroupTest)
	annotateDeployFlag(cmd, "test", deployGroupTest)
	annotateDeployFlag(cmd, "env", deployGroupTest)
	annotateDeployFlag(cmd, "parse", deployGroupTest)
	annotateDeployFlag(cmd, nonDagsFlag, deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-mount-path", deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-bundle-type", deployGroupNonDAG)
	annotateDeployFlag(cmd, "non-dags-local-path", deployGroupNonDAG)
	orderDeployFlagGroups(cmd)
	utils.MarkPreferredFlag(cmd.Flags(), "workspace", "workspace-id")
	applyPreferredFlagsIn(cmd)
	return cmd
}

func deployTests(parse, pytest, forceDeploy bool, pytestFile string) string {
	if pytest && pytestFile == "" {
		pytestFile = "all-tests"
	}

	if !parse && !pytest && !forceDeploy || parse && !pytest && !forceDeploy || parse && !pytest && forceDeploy {
		pytestFile = "parse"
	}

	if parse && pytest {
		pytestFile = "parse-and-all-tests"
	}

	return pytestFile
}

// manifestDeployIgnores are the flags astro deploy accepts that the manifest path never
// reads, mapped to what to do instead. Each was built for the 1.x deploy, and
// on the manifest path nothing carries it: not the Request built below, and not
// internal/deploy, which contains no reference to any of them.
//
// Refusing beats ignoring. A deploy that quietly skipped --pytest is a deploy
// someone believes ran their tests, and the flag having no effect is exactly
// the thing they cannot see. Porting them is not done yet.
//
// --force and --prompt are deliberately absent: the manifest path reads neither, but
// neither leaves a false belief behind. There is no uncommitted-changes gate on
// this path for --force to open, and a manifest deploy always asks, which is what
// --prompt was for. Both get the outcome the flag asked for.
//
// --build-secret has LEFT this list, which is the one entry that went the other
// way. It was blocked on tier 3, then on imagebuild passing a
// --secret; both have landed, so the flag is READ now. It still needs a project
// Dockerfile to be mounted into, and deployManifest refuses it without one — a refusal
// about what the project declares rather than about which version it is, gated on
// the flag being given rather than on a resolved value, since
// util.ResolveBuildSecrets also reads BUILD_SECRET_INPUT from the environment.
var manifestDeployIgnores = []struct {
	flag string
	do   string
}{
	{"pytest", "run your tests before deploying: uv run pytest && astro deploy"},
	{"parse", "check your DAGs before deploying: astro local check && astro deploy"},
	{"dags-path", "deploy from the project directory; a DAGs path other than dags/ is not supported yet"},
	{"dag-bundle-name", "named DAG bundles are not supported here yet"},
	{"test", "run your tests before deploying: uv run pytest <path> && astro deploy"},
	{"env", "this deploy runs no tests, so it reads no test env file; run your tests yourself with uv run pytest"},
	{"save", "this deploy always asks; set default = true on a link in [tool.astro.deployments] to move the cursor"},
	{"deployment-name", "name the target with --deployment, which takes a link name or a Deployment id"},
}

// refuseFlagsManifestDeployIgnores stops a manifest deploy that was given a flag it would
// silently drop.
func refuseFlagsManifestDeployIgnores(cmd *cobra.Command) error {
	for _, ignored := range manifestDeployIgnores {
		if cmd.Flags().Changed(ignored.flag) {
			return fmt.Errorf("--%s has no effect when deploying a project with a pyproject.toml ([tool.astro]): %s", ignored.flag, ignored.do)
		}
	}
	return nil
}

func deploy(cmd *cobra.Command, args []string) error {
	// Route by project type. A project with a pyproject.toml ([tool.astro])
	// takes the manifest deploy path; everything else runs the 1.x path below,
	// unchanged. project detection lives in internal/project.
	if project.HasManifest(config.WorkingPath) {
		return deployManifest(cmd, args)
	}

	deploymentID = ""

	// Get deploymentId from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return errors.New("cannot use --wait-time with --wait=false")
	}

	if deploymentID == "" || forcePrompt || workspaceID == "" {
		var err error
		workspaceID, err = coalesceWorkspace()
		if err != nil {
			return errors.Wrap(err, "failed to find a valid workspace")
		}
	}

	if dags && image {
		return errors.New("cannot use both --dags and --image together. Run 'astro deploy' to update both your image and dags")
	}

	if dagBundleName != "" && image {
		return errors.New("cannot use --dag-bundle-name with --image; named Dag bundles apply only to deploys that include Dags")
	}

	if cmd.Flags().Changed(imageNameFlag) {
		for _, f := range []string{"dags", "dags-path", "no-dags-base-dir", "pytest", "parse", "build-secret", "dag-bundle-name"} {
			if cmd.Flags().Changed(f) {
				return fmt.Errorf("cannot use --%s with --image-name; --image-name implies an image-only deploy", f)
			}
		}
	}

	if nonDags {
		return deployNonDagsBundle(cmd, args)
	}

	// Save deploymentId in config if specified
	if deploymentID != "" && saveDeployConfig {
		err := config.CFG.ProjectDeployment.SetProjectString(deploymentID)
		if err != nil {
			return errors.Wrap(err, "failed to save deployment id in config")
		}
	}

	// An error, not a printed note: returning nil here made a deploy that never
	// happened exit 0, so CI reported it as a success.
	if hasUncommittedChanges("") && !forceDeploy {
		// Not a usage mistake, so no usage block under the error.
		cmd.SilenceUsage = true
		return errUncommittedChanges
	}

	// case for astro deploy --dags whose default operation should be not running any tests
	if dags && !parse && !pytest {
		pytestFile = ""
	} else {
		pytestFile = deployTests(parse, pytest, forceDeploy, pytestFile)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	deployInput := astrodeploy.InputDeploy{
		Path:           config.WorkingPath,
		RuntimeID:      deploymentID,
		WsID:           workspaceID,
		WsIDFromFlag:   cmd.Flags().Changed("workspace-id"),
		Pytest:         pytestFile,
		EnvFile:        envFile,
		ImageName:      imageName,
		DeploymentName: deploymentName,
		Prompt:         forcePrompt,
		Dags:           dags,
		NoDagsBaseDir:  noDagsBaseDir,
		Image:          image,
		WaitForStatus:  waitForDeploy,
		WaitTime:       waitTime,
		Progress:       cmd.ErrOrStderr(),
		DagsPath:       dagsPath,
		Description:    deployDescription,
		BuildSecrets:   util.ResolveBuildSecrets(buildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		Force:          forceDeploy,
		DagBundleName:  dagBundleName,
	}

	return DeployImage(deployInput, astroV1Client, astroV1Alpha1Client)
}

func deployNonDagsBundle(cmd *cobra.Command, args []string) error {
	for _, f := range []string{"dags", "image", imageNameFlag, "dag-bundle-name", "pytest", "parse", "build-secret", "dags-path", "no-dags-base-dir"} {
		if cmd.Flags().Changed(f) {
			return fmt.Errorf("cannot use --%s with --non-dags; --non-dags performs a non-Dag bundle deploy", f)
		}
	}

	if nonDagsMountPath == "" {
		return errors.New("--non-dags-mount-path is required with --non-dags")
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

	withinAstroProject, err := config.IsWithinProjectDir(nonDagsBundlePath)
	if err != nil {
		return fmt.Errorf("failed to verify bundle path is not within an Astro project: %w", err)
	}
	if !withinAstroProject {
		withinAstroProject = isWithinManifestProject(nonDagsBundlePath)
	}
	if withinAstroProject {
		return errors.New("bundle path is within an Astro project. Non-Dag bundles must be a separate directory")
	}

	targetID, target, err := resolveBundleDeployment(args, workspaceID, deploymentName, "")
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
		AstroV1Client: astroV1Client,
	}
	res, err := DeployBundle(deployBundleInput)
	if err != nil {
		return err
	}
	// A 1.x project's deploy has no --output, so this is text: the line it has
	// always printed, then the wait.
	return publishThenWait(cmd, cliout.FormatText, waitForDeploy, res.DeploymentID, waitTime, func(error) error {
		return renderBundleUploaded(res.BundleVersion)(cmd.OutOrStdout())
	})
}

// deployManifest runs the manifest deploy path: load the manifest, gather flags and
// context, resolve the deployment, and run the deploy — dags-only, image-only,
// or both — then render the result. The manifest deploy's logic lives in internal/deploy; this
// is the cmd shim that parses, wires the transport, and prints.
func deployManifest(cmd *cobra.Command, args []string) error {
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
	cmd.SilenceUsage = true
	if rerr := refuseFlagsManifestDeployIgnores(cmd); rerr != nil {
		return manifestDeployErr(cmd, rerr)
	}

	out := cmd.OutOrStdout()

	m, err := manifest.Load(filepath.Join(config.WorkingPath, "pyproject.toml"))
	if err != nil {
		return manifestDeployErr(cmd, err)
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
	//     whenever the project happened to declare a Dockerfile. The 1.x body has a
	//     cross-flag guard listing build-secret, but a manifest deploy branches away
	//     before reaching it.
	//
	// One place, before anything is resolved or prompted for.
	if cmd.Flags().Changed("build-secret") {
		switch {
		case dags:
			return manifestDeployErr(cmd, errors.New("--build-secret has no effect with --dags: a dags-only deploy builds no image"))
		case imageName != "":
			return manifestDeployErr(cmd, errors.New("--build-secret has no effect with --image-name: the image is already built"))
		case m.Astro.Dockerfile == "":
			if err := util.CheckGeneratedBuildSecrets(buildSecrets); err != nil {
				return manifestDeployErr(cmd, err)
			}
		}
	}

	linkName := ""
	if len(args) > 0 {
		linkName = args[0]
	}

	login, err := loginForDeploy(cmd.Context(), m.Astro.LoginDomain())
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
	willPrompt := interactive && linkName == "" && manifestDeployment == ""
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
		Image:            image,
		ImageName:        imageName,
		Description:      deployDescription,
		Wait:             waitForDeploy,
		WaitTime:         waitTime,
		NoDagsBaseDir:    noDagsBaseDir,
		Interactive:      interactive,
		// The 1.x path's resolution, so a CI job setting BUILD_SECRET_INPUT
		// keeps working across the version boundary rather than silently
		// losing its secrets on the day the project converts. With neither the
		// flag nor the variable, the manifest's build-secrets apply.
		BuildSecrets: util.ResolveProjectBuildSecrets(buildSecrets, m.Astro.BuildSecretSpecs()),
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
	return renderManifestDeploy(out, format, &res)
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
}

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
func renderManifestDeploy(w io.Writer, format cliout.Format, res *manifestdeploy.Result) error {
	obj := deployJSON{
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
	// current reports the login is the current context's, which the 1.x path's
	// deployment picker assumes.
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

// manifestDeployer wires internal/deploy's transport seam to the 1.x path's cloud/deploy
// transport and the deployment selection flow.
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
		Ended:   input.Required(errors.New("a deploy must name the deployment it ships to: astro deploy <name> or --deployment <name>")),
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

// ResolveUnlinked runs the 1.x path's workspace-level pick/create flow and returns the
// chosen deployment id.
func (d manifestDeployer) ResolveUnlinked(workspaceID string) (string, error) {
	if !d.login.current {
		return "", fmt.Errorf("this project deploys to %[1]s, and the deployment picker lists only the current context's Deployments. Pass --deployment <id>, or run astro context switch %[1]s", d.login.context.Domain)
	}
	dep, err := deployment.GetDeployment(workspaceID, "", "", false, nil, d.login.client)
	if err != nil {
		return "", err
	}
	return dep.Id, nil
}

// DeployDags reuses the 1.x dags-only transport for the project's dags/.
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
