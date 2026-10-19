package astro

import (
	"bufio"
	"context"
	goerrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
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
	manifestDeployment string
	manifestWorkspace  string
	deployOutput       string
	deployExample      = `  # Deploy this project, picking the Deployment from a list
  astro deploy

  # Deploy to a given Deployment
  astro deploy <DEPLOYMENT_ID>

  # Deploy only the Dags
  astro deploy <DEPLOYMENT_ID> --dags

  # Deploy an image built on this machine, from any directory
  astro deploy <DEPLOYMENT_ID> --image-name <IMAGE_NAME>

  # Deploy a dbt project, or another non-Dag bundle, from its own directory
  astro deploy bundle <PATH> --deployment <DEPLOYMENT_ID>`

	buildSecrets = []string{}
)

const (
	deployWaitTime = 300 * time.Second

	imageNameFlag = "image-name"
)

func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy [DEPLOYMENT_ID]",
		Short:   "Ship this project's code to a Deployment",
		Long:    "Deploy your project to a Deployment on Astro. Run it in a project with a pyproject.toml ([tool.astro]): it builds the project into a Docker image and pushes that image, with the project's Dags, to Astronomer. A project in the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml) is converted with astro init, or deployed with Astro CLI 1.x. With --image-name it deploys an image built on this machine, from any directory. In Deployments with Remote Execution enabled, this only updates the Orchestration Plane components (the API Server and Scheduler). For all other components, use astro remote deploy instead. It does not include any metadata associated with your local Airflow environment. To deploy a dbt project or another non-Dag bundle, use astro deploy bundle.",
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
	addWorkspaceFlag(cmd.Flags(), &manifestWorkspace, "", "Workspace for the deploy, overriding the context")
	cmd.Flags().StringVar(&deployOutput, "output", string(cliout.FormatText), "Output format: text or json")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Add a description for more context on this deploy")
	utils.AddBuildSecretFlag(cmd.Flags(), &buildSecrets)

	annotateDeployFlag(cmd, "image", deployGroupImage)
	annotateDeployFlag(cmd, imageNameFlag, deployGroupImage)
	annotateDeployFlag(cmd, "build-secret", deployGroupImage)
	annotateDeployFlag(cmd, "dags", deployGroupDAG)
	annotateDeployFlag(cmd, "no-dags-base-dir", deployGroupDAG)
	orderDeployFlagGroups(cmd)
	// astro deploy bundle is the subcommand, so the word bundle is never a
	// Deployment argument: a link of that name is reached with --deployment.
	cmd.AddCommand(newDeployBundleCmd())
	applyPreferredFlagsIn(cmd)
	return cmd
}

// deploy ships a project with a pyproject.toml ([tool.astro]), the one kind
// of project v2 deploys (deployManifest), run from the project's root.
//
// The project decides, by utils.Locate's walk. In or below a
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
	// One walk routes the deploy and words its refusal.
	where := utils.Locate(config.WorkingPath)
	if where.AtRoot(config.WorkingPath) {
		return deployManifest(cmd, args, &where)
	}
	if _, err := cliout.ParseFormat(deployOutput); err != nil {
		return err
	}
	// The value, not whether the flag was given: an empty --image-name=
	// names no image, and would be a build.
	if imageName != "" && where.None() {
		return deployManifest(cmd, args, nil)
	}
	// Not a usage mistake, so no usage block under the error.
	cmd.SilenceUsage = true
	return utils.NoDeployableProject(where)
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
			return cliout.Usage(errors.New("cannot use --no-dags-base-dir with --image: an image-only deploy ships no Dags"))
		case imageName != "" && !inProject:
			return cliout.Usage(errors.New("cannot use --no-dags-base-dir with --image-name outside a pyproject.toml project: the image ships alone, with no Dags"))
		}
	}
	return nil
}

// errPickerOtherHost is the workspace picker refused on a login other than
// the current context's: it lists only the current context's Deployments.
func errPickerOtherHost(domain string) error {
	return fmt.Errorf("this project deploys to %[1]s, and the deployment picker lists only the current context's Deployments. Pass --deployment <id>, or run astro context switch %[1]s", domain)
}

// manifestHere is the manifest of the pyproject.toml project the working
// directory is the root of, as the walk loaded it, or nil when there is none
// or it did not load.
func manifestHere() *manifest.Manifest {
	return utils.Locate(config.WorkingPath).Loaded(config.WorkingPath)
}

func hasLink(m *manifest.Manifest, name string) bool {
	_, ok := m.Astro.Deployments[name]
	return ok
}

// deployManifest runs the manifest deploy path: load the manifest, gather flags and
// context, resolve the deployment, and run the deploy — dags-only, image-only,
// or both — then render the result. The manifest deploy's logic lives in internal/deploy; this
// is the cmd shim that parses, wires the transport, and prints.
//
// root is the walk's answer for the project the deploy runs at the root of,
// carrying its loaded manifest. It is nil for an --image-name deploy outside a
// pyproject.toml project: there is no manifest, so the target is a Deployment id (the
// argument or --deployment) or the workspace's pick, the login is the current
// context's, and the image ships alone, with no DAGs.
func deployManifest(cmd *cobra.Command, args []string, root *utils.Where) error {
	inProject := root != nil
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

	// The manifest the walk loaded, or why it did not: it is not read again.
	var m *manifest.Manifest
	if inProject {
		if root.LoadErr != nil {
			return manifestDeployErr(cmd, root.LoadErr)
		}
		m = root.Manifest
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

	// --workspace, or its 1.x spelling --workspace-id.
	overrideWorkspace := manifestWorkspace

	// A prompt has to be answered by someone, and json output has to stay a
	// stream a program can parse — a question on stderr with the run blocked on
	// stdin is not that. So json mode is non-interactive whatever stdin is, and
	// must name its target like any other script.
	interactive := format == cliout.FormatText && stdinIsTerminal()

	errOut := cmd.ErrOrStderr()
	// Every warning the deploy gives goes to stderr as it is given, and into
	// the result's warnings, so -o json carries the same ones.
	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
		fmt.Fprintf(errOut, "warning: %s\n", msg)
	}
	noteIgnoredDeploymentEnv(errOut, m, interactive, linkName, manifestDeployment)
	res, err := manifestdeploy.Run(manifestdeploy.Request{
		ProjectDir:       config.WorkingPath,
		Manifest:         m,
		LinkName:         linkName,
		Deployment:       manifestDeployment,
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
		Warn: warn,
		// Resolved once, when a deploy that builds first needs it, and
		// shared by the checks and the transport.
		Engine: onceEngine(astrodeploy.ResolveContainerEngine),
		// A Deployment picked from the workspace rather than from the links is
		// linked as `astro link add` would link it, once the deploy to it has
		// succeeded, so the next deploy offers it by name. Only an interactive
		// run picks one, so this is text mode.
		Link: func(l scaffold.Link) error {
			if err := scaffold.SaveLink(config.WorkingPath, nil, l); err != nil {
				return err
			}
			fmt.Fprintf(errOut, "linked %s to astro deployment %s in %s\n", l.Name, l.Deployment, project.Marker)
			// The save marks the project's former only link default = true;
			// say so, since it is a line the user did not ask to change.
			if after, err := manifest.Load(filepath.Join(config.WorkingPath, project.Marker)); err == nil {
				if kept := scaffold.NewlyDefault(m, after); kept != "" {
					fmt.Fprintf(errOut, "marked %s default = true: it was the project's only link, so its default, and stays so now there are two\n", kept)
				}
			}
			return nil
		},
		// Two lines. The first is the → line every resolving command prints,
		// once the target is settled, so a deploy says what it is about to act
		// on the way `astro af dags list` does. The second says an image build
		// is starting and can run for minutes; the build's own lines follow it
		// on stderr, through the transport's progress writer. It prints only once the
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
	if !inProject {
		warn(imageAloneWarning(&res, imageName))
	}
	return renderManifestDeploy(out, format, &res, warnings)
}

// onceEngine memoizes an engine resolver, so a deploy resolves its engine
// once however many steps ask.
var onceEngine = utils.OnceEngine

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

// noteIgnoredDeploymentEnv says, before the prompt, that an exported
// ASTRO_DEPLOYMENT does not steer it. The variable once moved the prompt's
// cursor, and someone who answers by habit should not find Enter now lands on
// another Deployment unannounced. Nothing is said when it names the project's
// default anyway, the row the prompt highlights, or when no prompt is coming:
// a run that cannot be asked, or one that names its target.
func noteIgnoredDeploymentEnv(w io.Writer, m *manifest.Manifest, interactive bool, named ...string) {
	env := os.Getenv(instances.EnvVar)
	// Outside a project (an --image-name deploy) there are no links for the
	// variable to have chosen among.
	if m == nil || env == "" || !interactive || slices.ContainsFunc(named, func(n string) bool { return n != "" }) {
		return
	}
	def, _, ok := manifest.DefaultLink(m.Astro.Deployments)
	switch {
	case ok && def == env:
		return
	case ok:
		fmt.Fprintf(w, "note: %s=%s chooses what the query commands read; astro deploy asks, with the project's default, %s, preselected\n", instances.EnvVar, env, def)
	default:
		fmt.Fprintf(w, "note: %s=%s chooses what the query commands read; astro deploy asks, and preselects nothing without a default link\n", instances.EnvVar, env)
	}
}

// stdinIsTerminal reports whether this run can be asked which deployment to
// ship to. It is a var rather than the call itself so a test can drive the
// prompt without a pty.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// deployJSON is the single object `astro deploy --output json` emits when a manifest
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
	// DagsInImage is set on an image-and-dag deploy to a Deployment with DAG
	// deploys off, which uploads no DAGs: the Deployment runs those inside
	// the image.
	DagsInImage bool `json:"dags_in_image,omitempty"`
	// RemoteExecution is set on an image-and-dag deploy to a
	// remote-execution Deployment with DAG deploys off, which uploads no
	// DAGs.
	RemoteExecution bool           `json:"remote_execution,omitempty"`
	RuntimeVersion  string         `json:"runtime_version,omitempty"`
	URL             string         `json:"url,omitempty"`
	Git             *deployGitJSON `json:"git,omitempty"`
	// Warnings are what the deploy warned about without failing, every one
	// that text prints on stderr (less the "warning: " prefix): the DAG and
	// build checks', a link it could not save, an image shipped alone.
	Warnings []string `json:"warnings,omitempty"`
}

// warningDagsNotUpdated is the warning for an --image-name deploy outside a
// project to a Deployment that takes DAG deploys: the image changed, its DAGs
// did not.
const warningDagsNotUpdated = "this deploy shipped the image alone, so Deployment %s keeps the Dags it had. " +
	"To update them, run astro deploy %s --dags from the project directory"

// warningDagsInImage is the same deploy to a Deployment that takes no DAG
// deploys: it runs the DAGs inside the image, which no project here put
// there. APC's deploy warns the same (warningImageNameDagsInImage).
const warningDagsInImage = "Deployment %s runs the Dags inside the image %s; no dags folder was deployed. " +
	"An image astro package built without a dockerfile declared under [tool.astro] contains none"

// imageAloneWarning is the warning an --image-name deploy outside a project
// gives: the image shipped alone. A Deployment that takes DAG deploys keeps
// the DAGs it had; a remote-execution one with them off runs the DAGs its
// remote agents have, which this deploy did not touch; any other runs
// whatever DAGs the image carries, which may be none.
func imageAloneWarning(res *manifestdeploy.Result, imageName string) string {
	switch {
	case res.DagDeployEnabled:
		return fmt.Sprintf(warningDagsNotUpdated, res.DeploymentID, res.DeploymentID)
	case res.RemoteExecutionEnabled:
		return fmt.Sprintf(warningRemoteDagsNotUpdated, res.DeploymentID)
	default:
		return fmt.Sprintf(warningDagsInImage, res.DeploymentID, imageName)
	}
}

// warningRemoteDagsNotUpdated is the same deploy to a remote-execution
// Deployment with DAG deploys off: the image updates its orchestration plane,
// and its DAGs are where its remote agents keep them.
const warningRemoteDagsNotUpdated = "this deploy shipped the image alone, and Deployment %s runs remote execution with Dag deploys off, " +
	"so its Dags are the ones its remote agents run, which this deploy did not change"

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
		DagsInImage:      res.DagsInImage,
		RemoteExecution:  res.RemoteExecution,
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
			fmt.Fprintf(b, "Deployed Dags (version %s) to %s.\n", res.DagTarballVersion, target)
		case "image-only":
			fmt.Fprintf(b, "Deployed image (tag %s) to %s.\n", res.ImageTag, target)
		case "image-and-dag":
			if res.DagsInImage {
				fmt.Fprintf(b, "Deployed image (tag %s) to %s, with the Dags inside the image: its Dag deploys are off, so no Dags were uploaded.\n", res.ImageTag, target)
				break
			}
			if res.RemoteExecution {
				fmt.Fprintf(b, "Deployed image (tag %s) to %s. It runs remote execution with Dag deploys off, so no Dags were uploaded.\n", res.ImageTag, target)
				break
			}
			fallthrough
		default:
			fmt.Fprintf(b, "Deployed image (tag %s) and Dags (version %s) to %s.\n", res.ImageTag, res.DagTarballVersion, target)
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

// ConfirmTarget asks which deployment to ship to, on stderr, as every
// pyproject.toml project's deploy asks (utils.ConfirmDeployTarget).
func (d manifestDeployer) ConfirmTarget(choices []manifestdeploy.Choice, defaultLink string) (string, error) {
	return utils.ConfirmDeployTarget(choices, defaultLink, d.in, d.errOut)
}

// ListDeployments lists a workspace's Deployments under the deploy's login, for
// the prompt to offer the ones the project does not link yet.
//
// The prompt waits on it, so it gives up after listDeploymentsTimeout and the
// prompt offers the links alone. The list takes no context to cancel, so a
// call that times out finishes in the background and is dropped.
func (d manifestDeployer) ListDeployments(workspaceID string) ([]manifestdeploy.Deployment, error) {
	type listed struct {
		deps []astrov1.Deployment
		err  error
	}
	done := make(chan listed, 1)
	list, org, client := deployment.ListDeployments, d.login.context.Organization, d.login.client
	go func() {
		deps, err := list(workspaceID, org, client)
		done <- listed{deps, err}
	}()
	var res listed
	select {
	case res = <-done:
	case <-time.After(listDeploymentsTimeout):
		return nil, fmt.Errorf("no answer after %s", listDeploymentsTimeout)
	}
	if res.err != nil {
		return nil, res.err
	}
	out := make([]manifestdeploy.Deployment, 0, len(res.deps))
	for i := range res.deps {
		out = append(out, manifestdeploy.Deployment{ID: res.deps[i].Id, Name: res.deps[i].Name, WorkspaceID: res.deps[i].WorkspaceId})
	}
	return out, nil
}

// listDeploymentsTimeout bounds how long the deploy prompt waits for the
// workspace's Deployments. A var so a test can shorten it.
var listDeploymentsTimeout = 15 * time.Second

// ResolveUnlinked runs the workspace-level pick/create flow and returns the
// chosen Deployment.
func (d manifestDeployer) ResolveUnlinked(workspaceID string) (manifestdeploy.Deployment, error) {
	if !d.login.current {
		return manifestdeploy.Deployment{}, errPickerOtherHost(d.login.context.Domain)
	}
	dep, err := deployment.GetDeployment(workspaceID, "", "", false, nil, d.login.client)
	if err != nil {
		return manifestdeploy.Deployment{}, err
	}
	return manifestdeploy.Deployment{ID: dep.Id, Name: dep.Name, WorkspaceID: dep.WorkspaceId}, nil
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
		ImageAlone:    in.ImageAlone,
		Warn:          in.Warn,
		Survey:        in.Survey,
		Engine:        in.Engine,
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
		WorkspaceID:            res.WorkspaceID,
		RuntimeVersion:         res.RuntimeVersion,
		ImageTag:               res.ImageTag,
		DagTarballVersion:      res.DagTarballVersion,
		DagsInImage:            res.DagsInImage,
		RemoteExecution:        res.RemoteExecution,
		URL:                    res.URL,
		Git:                    toManifestDeployGit(res.Git),
		DagDeployEnabled:       res.DagDeployEnabled,
		RemoteExecutionEnabled: res.RemoteExecutionEnabled,
	}, nil
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
