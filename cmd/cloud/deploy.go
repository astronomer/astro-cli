package cloud

import (
	"bufio"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	v2deploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	forceDeploy       bool
	forcePrompt       bool
	saveDeployConfig  bool
	pytest            bool
	parse             bool
	dags              bool
	waitForDeploy     bool
	waitTime          time.Duration
	image             bool
	dagsPath          string
	pytestFile        string
	envFile           string
	imageName         string
	deploymentName    string
	deployDescription string
	noDagsBaseDir     bool
	dagBundleName     string
	nonDags           bool
	nonDagsMountPath  string
	nonDagsBundleType string
	nonDagsBundlePath string
	v2Deployment      string
	v2Workspace       string
	deployOutput      string
	deployExample     = `
Specify the ID of the Deployment on Astronomer you would like to deploy this project to:

  $ astro deploy <deployment ID>

Menu will be presented if you do not specify a deployment ID:

  $ astro deploy
`

	DeployImage      = cloud.Deploy
	EnsureProjectDir = utils.EnsureProjectDir
	buildSecrets     = []string{}
)

const (
	registryUncommitedChangesMsg = "Project directory has uncommitted changes, use `astro deploy [deployment-id] -f` to force deploy."

	deployWaitTime = 300 * time.Second

	imageNameFlag = "image-name"
	nonDagsFlag   = "non-dags"
	dagsPathFlag  = "dags-path"
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy DEPLOYMENT-ID",
		Short: "Ship this project's code to a Deployment",
		Long:  "Deploy your project to a Deployment on Astro. This command bundles your project files into a Docker image and pushes that Docker image to Astronomer. In Deployments with Remote Execution enabled, this only updates the Orchestration Plane components (the API Server and Scheduler). For all other components, use `astro remote deploy` instead. It does not include any metadata associated with your local Airflow environment.",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed(imageNameFlag) || cmd.Flags().Changed(nonDagsFlag) {
				return nil
			}
			// A v2 project has no .astro/config.yaml, so the v1 EnsureProjectDir
			// check would reject it. The v2 path loads and validates the manifest
			// itself, so skip the v1 check and let deploy() route.
			if v2deploy.IsV2Project(config.WorkingPath) {
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
	cmd.Flags().BoolVarP(&dags, "dags", "d", false, "Push only DAGs to your Astro Deployment")
	cmd.Flags().BoolVar(&noDagsBaseDir, "no-dags-base-dir", false, "Exclude the dags directory prefix from the bundle. Use for Airflow 3.x deployments where sys.path includes the bundle root")
	cmd.Flags().StringVar(&dagBundleName, "dag-bundle-name", "", "Deploy DAGs to a named DAG bundle on the Deployment instead of the default bundle. Requires Airflow 3, and the bundle must already exist on the Deployment")
	cmd.Flags().MarkHidden("dag-bundle-name") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().BoolVarP(&image, "image", "", false, "Push only an image to your Astro Deployment. If you have DAG Deploy enabled your DAGs will not be affected.")
	cmd.Flags().StringVar(&dagsPath, dagsPathFlag, "", "If set deploy dags from this path instead of the dags from working directory")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to deploy to")
	cmd.Flags().BoolVar(&parse, "parse", false, "Succeed only if all DAGs in your Astro project parse without errors")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTime, "wait-time", deployWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cmd.Flags().MarkHidden("dags-path") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	// No -d shorthand: on this command -d has meant --dags since v1, and moving
	// it would turn `astro deploy -d` from a DAG-only deploy into a target
	// selector. -d/--deployment is the spelling everywhere the letter is free.
	cmd.Flags().StringVar(&v2Deployment, "deployment", "", "Deployment to deploy to: a link name from the manifest, or a Deployment id. For v2 projects (a pyproject.toml with [tool.astro])")
	cmd.Flags().StringVar(&v2Workspace, "workspace", "", "Workspace for the deploy, overriding the context. For v2 projects (a pyproject.toml with [tool.astro])")
	cmd.Flags().StringVar(&deployOutput, "output", string(formatText), "Output format for v2 projects: text or json")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Add a description for more context on this deploy")
	utils.AddBuildSecretFlags(cmd.Flags(), &buildSecrets)
	cmd.Flags().Bool("force-upgrade-to-af3", false, "This flag is no longer required for Airflow 2 to Airflow 3 upgrades. Support will be removed in a future release.")
	cmd.Flags().MarkDeprecated("force-upgrade-to-af3", "this flag is no longer required for Airflow 2 to Airflow 3 upgrades. Support will be removed in a future release.") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().BoolVar(&nonDags, nonDagsFlag, false, "Deploy a non-DAG bundle from a separate directory, instead of your Astro project. Requires --non-dags-mount-path")
	cmd.Flags().StringVar(&nonDagsMountPath, "non-dags-mount-path", "", "Path to mount the non-DAG bundle in Airflow, for reference by DAGs. Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundleType, "non-dags-bundle-type", "none", "Free-form label identifying the kind of non-DAG bundle (e.g. dbt). Any value is accepted. Defaults to \"none\". Used with --non-dags")
	cmd.Flags().StringVar(&nonDagsBundlePath, "non-dags-local-path", "", "Path to the non-DAG bundle to deploy. Default current directory. Used with --non-dags")
	cmd.Flags().MarkHidden(nonDagsFlag)            //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().MarkHidden("non-dags-mount-path")  //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().MarkHidden("non-dags-bundle-type") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().MarkHidden("non-dags-local-path")  //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name

	annotateDeployFlag(cmd, "image", "image")
	annotateDeployFlag(cmd, imageNameFlag, "image")
	annotateDeployFlag(cmd, "build-secret", "image")
	annotateDeployFlag(cmd, "dags", "dag")
	annotateDeployFlag(cmd, "no-dags-base-dir", "dag")
	annotateDeployFlag(cmd, "dag-bundle-name", "dag")
	annotateDeployFlag(cmd, dagsPathFlag, "dag")
	annotateDeployFlag(cmd, "pytest", "test")
	annotateDeployFlag(cmd, "test", "test")
	annotateDeployFlag(cmd, "env", "test")
	annotateDeployFlag(cmd, "parse", "test")
	annotateDeployFlag(cmd, nonDagsFlag, "non-dags")
	annotateDeployFlag(cmd, "non-dags-mount-path", "non-dags")
	annotateDeployFlag(cmd, "non-dags-bundle-type", "non-dags")
	annotateDeployFlag(cmd, "non-dags-local-path", "non-dags")
	cmd.SetUsageTemplate(deployFlagsUsageTemplate)
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

func deploy(cmd *cobra.Command, args []string) error {
	// Route by project type. A v2 project (a pyproject.toml with [tool.astro])
	// takes the new v2 deploy path; everything else runs the v1 path below,
	// unchanged. project detection lives in internal/deploy.
	if v2deploy.IsV2Project(config.WorkingPath) {
		return deployV2(cmd, args)
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
		return errors.New("cannot use --dag-bundle-name with --image; named DAG bundles apply only to deploys that include DAGs")
	}

	if cmd.Flags().Changed(imageNameFlag) {
		for _, f := range []string{"dags", "dags-path", "no-dags-base-dir", "pytest", "parse", "build-secret", "build-secrets", "dag-bundle-name"} {
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

	if git.HasUncommittedChanges("") && !forceDeploy {
		fmt.Println(registryUncommitedChangesMsg)
		return nil
	}

	// case for astro deploy --dags whose default operation should be not running any tests
	if dags && !parse && !pytest {
		pytestFile = ""
	} else {
		pytestFile = deployTests(parse, pytest, forceDeploy, pytestFile)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	deployInput := cloud.InputDeploy{
		Path:           config.WorkingPath,
		RuntimeID:      deploymentID,
		WsID:           workspaceID,
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
		DagsPath:       dagsPath,
		Description:    deployDescription,
		BuildSecrets:   util.ResolveBuildSecrets(buildSecrets, os.Getenv("BUILD_SECRET_INPUT")),
		Force:          forceDeploy,
		DagBundleName:  dagBundleName,
	}

	return DeployImage(deployInput, astroV1Client, astroV1Alpha1Client)
}

func deployNonDagsBundle(cmd *cobra.Command, args []string) error {
	for _, f := range []string{"dags", "image", imageNameFlag, "dag-bundle-name", "pytest", "parse", "build-secret", "build-secrets", "dags-path", "no-dags-base-dir"} {
		if cmd.Flags().Changed(f) {
			return fmt.Errorf("cannot use --%s with --non-dags; --non-dags performs a non-DAG bundle deploy", f)
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
		withinAstroProject = isWithinV2Project(nonDagsBundlePath)
	}
	if withinAstroProject {
		return errors.New("bundle path is within an Astro project. Non-DAG bundles must be a separate directory")
	}

	targetDeploymentID, err := resolveDeploymentIDFromArgsFlags(args, workspaceID, deploymentName)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true

	deployBundleInput := &cloud.DeployBundleInput{
		BundlePath:    nonDagsBundlePath,
		MountPath:     nonDagsMountPath,
		DeploymentID:  targetDeploymentID,
		BundleType:    nonDagsBundleType,
		Description:   deployDescription,
		Wait:          waitForDeploy,
		WaitTime:      waitTime,
		AstroV1Client: astroV1Client,
	}
	return DeployBundle(deployBundleInput)
}

// deployV2 runs the v2 deploy path: load the manifest, gather flags and
// context, resolve the deployment, and run the deploy — dags-only, image-only,
// or both — then render the result. The v2 logic lives in internal/deploy; this
// is the cmd shim that parses, wires the transport, and prints.
func deployV2(cmd *cobra.Command, args []string) error {
	format, err := parseDeployFormat(deployOutput)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()

	m, err := manifest.Load(filepath.Join(config.WorkingPath, "pyproject.toml"))
	if err != nil {
		return deployV2Err(cmd, format, err)
	}

	linkName := ""
	if len(args) > 0 {
		linkName = args[0]
	}

	// The workspace from the current context is the fallback when neither
	// --workspace nor a link carries one. An empty context is not an error here;
	// internal/deploy decides when a workspace is actually required.
	contextWorkspace, _ := workspace.GetCurrentWorkspace() //nolint:errcheck // absent context is handled downstream

	// --workspace wins over the legacy --workspace-id when both are set.
	overrideWorkspace := v2Workspace
	if overrideWorkspace == "" {
		overrideWorkspace = workspaceID
	}

	cmd.SilenceUsage = true

	// A prompt has to be answered by someone, and json output has to stay a
	// stream a program can parse — a question on stderr with the run blocked on
	// stdin is not that. So json mode is non-interactive whatever stdin is, and
	// must name its target like any other script.
	interactive := format == formatText && stdinIsTerminal()

	// Only a run that is going to ask has any use for the ambient layers, and
	// reading the pin is not free — it creates the project's state directory.
	willPrompt := interactive && linkName == "" && v2Deployment == ""
	preselect, preselectFrom := deployPreselect(config.WorkingPath, willPrompt)

	errOut := cmd.ErrOrStderr()
	res, err := v2deploy.Run(v2deploy.Request{
		ProjectDir:       config.WorkingPath,
		Manifest:         m,
		LinkName:         linkName,
		Deployment:       v2Deployment,
		Preselect:        preselect,
		PreselectFrom:    preselectFrom,
		WorkspaceID:      overrideWorkspace,
		ContextWorkspace: contextWorkspace,
		DagsOnly:         dags,
		Image:            image,
		ImageName:        imageName,
		Description:      deployDescription,
		Wait:             waitForDeploy,
		WaitTime:         waitTime,
		NoDagsBaseDir:    noDagsBaseDir,
		Interactive:      interactive,
		// Two lines, once the target is settled and before anything is built.
		// The first is the → line every resolving command prints, so a deploy
		// says what it is about to act on the way `astro af dags list` does. The
		// second says an image build can run for minutes with no transport
		// output yet; the transport itself stays silent (v2 layer rules).
		//
		// Both fire after the target is settled, so a deploy refused at the
		// prompt claims nothing. They go to stderr and stdout respectively —
		// the announce line is context, the progress line is this command's
		// own output — and json mode drops both, so the single result object is
		// all it adds.
		Announce: func(target v2deploy.Target) {
			if format != formatText {
				return
			}
			announceDeployTarget(errOut, target)
			if dags {
				// A dags-only deploy builds nothing, so there is no wait to
				// explain.
				return
			}
			if imageName != "" {
				fmt.Fprintf(out, "Deploying prebuilt image %s...\n", imageName)
			} else {
				fmt.Fprintln(out, "Building your project image, this can take a few minutes...")
			}
		},
	}, newV2Deployer(astroV1Client, cmd.InOrStdin(), errOut))
	if err != nil {
		return deployV2Err(cmd, format, err)
	}

	return renderV2Deploy(out, format, &res)
}

// announceDeployTarget prints the one line every resolving command puts on
// stderr before it acts, so a deploy's target is never invisible either. It
// matches cmd/local's announceInstance: the name, then what the name does not
// already say. A deployment named by id has nothing to add.
func announceDeployTarget(w io.Writer, target v2deploy.Target) {
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
	return state.Instance, v2deploy.PinnedBy
}

// deployFormat selects how the v2 deploy path renders its result.
type deployFormat string

const (
	formatText deployFormat = "text"
	formatJSON deployFormat = "json"
)

// parseDeployFormat validates the --output value for the v2 deploy path.
func parseDeployFormat(s string) (deployFormat, error) {
	switch deployFormat(s) {
	case formatText:
		return formatText, nil
	case formatJSON:
		return formatJSON, nil
	default:
		return "", fmt.Errorf("unknown output format %q (supported: text, json)", s)
	}
}

// deployJSON is the single object `astro deploy --output json` emits when a v2
// deploy finishes. Fields that do not apply to a deploy kind are omitted: a
// dags-only deploy carries no image_tag, an image-only deploy no
// dag_bundle_version.
type deployJSON struct {
	Deployment string `json:"deployment"`
	// Link is the manifest link the deploy resolved to, omitted when the target
	// was named by id. It is what the person typed and what their teammates
	// call it; the id alone makes a consumer look it up again.
	Link             string `json:"link,omitempty"`
	Workspace        string `json:"workspace"`
	Type             string `json:"type"`
	ImageTag         string `json:"image_tag,omitempty"`
	DagBundleVersion string `json:"dag_bundle_version,omitempty"`
	RuntimeVersion   string `json:"runtime_version,omitempty"`
	URL              string `json:"url,omitempty"`
}

// renderV2Deploy writes a finished v2 deploy: one JSON object in json mode, a
// plain summary in text mode. Both render the same Result, so the two modes
// never drift.
func renderV2Deploy(w io.Writer, format deployFormat, res *v2deploy.Result) error {
	if format == formatJSON {
		return json.NewEncoder(w).Encode(deployJSON{
			Deployment:       res.DeploymentID,
			Link:             res.LinkName,
			Workspace:        res.WorkspaceID,
			Type:             res.Type,
			ImageTag:         res.ImageTag,
			DagBundleVersion: res.DagTarballVersion,
			RuntimeVersion:   res.RuntimeVersion,
			URL:              res.URL,
		})
	}
	target := deployTargetName(res)
	switch res.Type {
	case "dag-only":
		fmt.Fprintf(w, "Deployed DAGs (version %s) to %s.\n", res.DagTarballVersion, target)
	case "image-only":
		fmt.Fprintf(w, "Deployed image (tag %s) to %s.\n", res.ImageTag, target)
	default: // image-and-dag
		fmt.Fprintf(w, "Deployed image (tag %s) and DAGs (version %s) to %s.\n", res.ImageTag, res.DagTarballVersion, target)
	}
	if res.URL != "" {
		fmt.Fprintf(w, "Deployment: %s\n", res.URL)
	}
	return nil
}

// deployTargetName is what the summary line calls where the code went: the link
// name the user chose, with the id in tow, because the id alone is the one thing
// nobody recognizes at a glance.
func deployTargetName(res *v2deploy.Result) string {
	if res.LinkName == "" {
		return "deployment " + res.DeploymentID
	}
	return fmt.Sprintf("%s (deployment %s)", res.LinkName, res.DeploymentID)
}

// deployV2Err renders a v2 deploy failure. In json mode it writes the
// {"error","code"} object to stdout and silences cobra's error and usage
// output so the object is the only thing on the streams; in text mode it
// returns the error for cobra to print. It returns the error either way, so
// the process still exits non-zero.
func deployV2Err(cmd *cobra.Command, format deployFormat, err error) error {
	if goerrors.Is(err, v2deploy.ErrAborted) {
		// The user was asked and said no. Reading their own answer back at them
		// as "Error: no deployment selected" adds nothing; the exit code carries
		// the whole message. Only an interactive run can reach here, so this
		// never eats the object json mode promises.
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return err
	}
	if format == formatJSON {
		//nolint:errcheck // the command already failed; a write error changes nothing
		json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Error string `json:"error"`
			Code  int    `json:"code"`
		}{Error: err.Error(), Code: 1})
		// A manifest or selection failure can return before deployV2 sets
		// SilenceUsage, so set it here too — json mode must not dump the usage
		// block onto stderr alongside the object.
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
	}
	return err
}

// newV2Deployer builds the transport the v2 deploy path drives. It is a var so a
// test can swap in a fake and exercise the whole cmd path — flag parsing,
// selection, and rendering — with no real daemon, registry, or API.
var newV2Deployer = func(client astrov1.APIClient, in io.Reader, errOut io.Writer) v2deploy.Deployer {
	return v2Deployer{client: client, in: in, errOut: errOut}
}

// v2Deployer wires internal/deploy's transport seam to the v1 cloud/deploy
// transport and the deployment selection flow.
type v2Deployer struct {
	client astrov1.APIClient
	// in and errOut carry the deploy prompt. It asks on stderr and reads stdin,
	// so stdout stays the deploy's own output.
	in     io.Reader
	errOut io.Writer
}

// deployPromptAttempts bounds the re-asking, so a stdin that answers but never
// answers usefully ends rather than loops. It matches the query surface's
// picker (cmd/local).
const deployPromptAttempts = 3

// ConfirmTarget asks which deployment to ship to. Every interactive deploy that
// did not name its target comes through here — a pin, ASTRO_DEPLOYMENT, or a
// `default = true` marker moves the cursor and never skips the question
// (docs/v2-instances.md decision 2).
//
// The highlight is labeled with what put it there, not with one word for all
// three: a cursor sitting on an entry because a variable is exported in this
// shell is a different fact from one sitting there because the committed
// manifest says so, and only the reader can tell which they meant.
//
// It takes a name or a number, and Enter takes the highlighted entry when there
// is one. With nothing highlighted there is no default on Enter: the safe answer
// to "where should I ship this code" is never one the CLI picked by itself.
func (d v2Deployer) ConfirmTarget(choices []v2deploy.Choice, preselect v2deploy.Preselect) (string, error) {
	fmt.Fprintln(d.errOut, "Deploy to which deployment?")
	chosen := 0
	for i, choice := range choices {
		marker := ""
		if preselect.Name != "" && choice.Name == preselect.Name {
			marker, chosen = "  ← "+preselect.From, i+1
		}
		fmt.Fprintf(d.errOut, "  %d) %s (%s)%s\n", i+1, choice.Name, choice.Where, marker)
	}
	prompt := fmt.Sprintf("Choose 1-%d: ", len(choices))
	if chosen > 0 {
		prompt = fmt.Sprintf("Choose 1-%d [%d]: ", len(choices), chosen)
	}
	in := bufio.NewReader(d.in)
	for attempt := 0; attempt < deployPromptAttempts; attempt++ {
		fmt.Fprintf(d.errOut, "%s", prompt)
		line, err := in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if answer == "" && chosen > 0 && err == nil {
			return preselect.Name, nil
		}
		if err != nil && answer == "" {
			if goerrors.Is(err, io.EOF) {
				return "", errors.New("a deploy must name the deployment it ships to: `astro deploy <name>` or --deployment <name>")
			}
			return "", err
		}
		if name, ok := matchDeployChoice(choices, answer); ok {
			return name, nil
		}
		fmt.Fprintf(d.errOut, "Not one of the choices. ")
	}
	// Asked three times, told three times that the answer was not one of the
	// choices, and still no pick. The prompt has already said everything there
	// is to say, so this ends quietly — that is what the sentinel is for.
	return "", v2deploy.ErrAborted
}

// matchDeployChoice reads an answer as a name or a number, names first. A link
// may legally be called "2", and a project with one offered second in the list
// would otherwise read "2" as "the second entry" and ship somewhere else
// entirely. What the user typed is what they meant; the numbers are only a
// shorthand for names nobody wants to retype.
func matchDeployChoice(choices []v2deploy.Choice, answer string) (string, bool) {
	for _, choice := range choices {
		if choice.Name == answer {
			return choice.Name, true
		}
	}
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(choices) {
		return choices[n-1].Name, true
	}
	return "", false
}

// ResolveUnlinked runs v1's workspace-level pick/create flow and returns the
// chosen deployment id.
func (d v2Deployer) ResolveUnlinked(workspaceID string) (string, error) {
	dep, err := deployment.GetDeployment(workspaceID, "", "", false, nil, d.client)
	if err != nil {
		return "", err
	}
	return dep.Id, nil
}

// DeployDags reuses the v1 dags-only transport for the v2 project's dags/.
func (d v2Deployer) DeployDags(in *v2deploy.DagDeploy) (v2deploy.DagResult, error) {
	res, err := cloud.DeployDagsV2(cloud.DagDeployV2Input{
		ProjectDir:    in.ProjectDir,
		DeploymentID:  in.DeploymentID,
		Description:   in.Description,
		NoDagsBaseDir: in.NoDagsBaseDir,
		Wait:          in.Wait,
		WaitTime:      in.WaitTime,
	}, d.client)
	if err != nil {
		return v2deploy.DagResult{}, err
	}
	return v2deploy.DagResult{
		WorkspaceID:       res.WorkspaceID,
		RuntimeVersion:    res.RuntimeVersion,
		DagTarballVersion: res.DagTarballVersion,
		URL:               res.URL,
	}, nil
}

// DeployImage builds or adopts the project image and ships it through the
// cloud/deploy transport.
func (d v2Deployer) DeployImage(in *v2deploy.ImageDeploy) (v2deploy.ImageResult, error) {
	res, err := cloud.DeployImageV2(cloud.ImageDeployV2Input{
		ProjectDir:     in.ProjectDir,
		DeploymentID:   in.DeploymentID,
		AirflowVersion: in.AirflowVersion,
		Dependencies:   in.Dependencies,
		Packages:       in.Packages,
		ImageName:      in.ImageName,
		IncludeDags:    in.IncludeDags,
		Description:    in.Description,
		NoDagsBaseDir:  in.NoDagsBaseDir,
		Wait:           in.Wait,
		WaitTime:       in.WaitTime,
	}, d.client)
	if err != nil {
		return v2deploy.ImageResult{}, err
	}
	return v2deploy.ImageResult{
		WorkspaceID:       res.WorkspaceID,
		RuntimeVersion:    res.RuntimeVersion,
		ImageTag:          res.ImageTag,
		DagTarballVersion: res.DagTarballVersion,
		URL:               res.URL,
	}, nil
}

// isWithinV2Project reports whether path sits at or inside a v2 project.
//
// config.IsWithinProjectDir only knows the v1 marker, .astro/config.yaml, so it
// answers false at every level of a v2 project. That left both containment
// refusals in this package unreachable for exactly the projects v2 users have:
// a dbt project or a non-DAG bundle nested inside one shipped where the check
// meant to stop it.
func isWithinV2Project(path string) bool {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	for dir := abs; ; {
		if v2deploy.IsV2Project(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
