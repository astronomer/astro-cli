// Package local is the core command tree: `astro local *`, `astro init`, the
// root aliases `astro start/stop/logs`, and the `astro dev` removal stub.
// It follows the cmd/ layer rules in docs/architecture.md: parse flags,
// call one function, render output. All process state (stdio, the runtime,
// the working directory) arrives through Deps, built once in main; nothing
// here reads config at import time or holds mutable package state.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/containercfg"
	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/imageinstall"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/internal/proxydaemon"
	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// Deps is everything the core commands need from the process. The composition
// root (the shell root, cmd/root.go) builds it once through NewDeps and hands it
// down.
type Deps struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Runtime is the pkg/localrt surface the commands call.
	Runtime Runtime
	// ProxyDir is where the proxy routes and the daemon's record live
	// (proxydaemon.Dir), resolved once per run. Empty when there is no home
	// directory: HomeErr then says why, and Runtime is a stand-in returning it.
	ProxyDir string
	HomeErr  error

	// Checks runs `astro local check`'s DAG parse against the project venv.
	Checks checks.Parser

	// CheckVenv parses DAGs in a scratch venv for a `--target` pre-flight
	// check. Production is the same *checks.VenvRunner as Checks; a test
	// injects a stub.
	CheckVenv checks.TargetParser

	// Provisioner builds and caches the scratch venvs a target check needs.
	// nil uses the uv-backed production provisioner; a test injects a fake.
	Provisioner func(ctx context.Context) (checks.Provisioner, error)

	// WorkingDir resolves the project path. Commands never call os.Getwd
	// themselves so tests can pin it.
	WorkingDir func() (string, error)

	// OpenURL opens a URL in the user's browser (`astro local open`).
	OpenURL func(url string) error

	// WorkspaceClients builds the client the Environment Manager read-through
	// provider reads workspace-source values with, from the stored login for
	// the manifest's domain — not the current context, so a production-linked
	// project keeps reading production while the CLI is switched to dev. Nil
	// turns workspace resolution off. A test injects a fake.
	WorkspaceClients emenv.ClientFactory

	// Session hands instance resolution the bearer of the login for domain —
	// the project's Astro host, or the current context when it names none — for
	// a link that proves itself with the astro auth method. It is a seam
	// because the read touches config/, which this tree never imports directly;
	// nil reads as logged out.
	Session func(ctx context.Context, domain string) (string, error)

	// Locator builds, for the project's Astro host, what turns a coordinate
	// link into an Airflow base URL: a Deployment's web server from that host's
	// control plane, a Composer environment's Airflow URI from the Composer
	// API. It is a seam for the same reason Session is — the lookups touch
	// config/ and the cloud clients — and nil means a coordinate link cannot be
	// reached, which is what a test that declares none wants.
	Locator func(domain string) instances.Locator

	// CurrentWorkspace, PickDeployment and PickWorkspace are the Astro pickers
	// `astro link` asks with when a run can be asked and an argument is
	// missing: the current login's workspace, then the Deployment and workspace
	// lists `astro deploy` and `astro workspace switch` offer. The root wires
	// them over the v1 client, which this tree cannot import. nil means the
	// run cannot pick, and the missing argument is an error.
	CurrentWorkspace func() string
	// PickDeployment leaves out the Deployments in linked, which maps a
	// Deployment ID to the link naming it, and reports ErrAllDeploymentsLinked
	// when that leaves none to offer.
	PickDeployment func(workspaceID string, linked map[string]string) (PickedDeployment, error)
	PickWorkspace  func() (string, error)

	// APC is set by the root when the current context is Astro Private
	// Cloud's, and nil under Astro. `astro link add` then links an APC
	// Deployment by default (target = "apc"), on this context's domain.
	APC *APCLinking

	// Interactive reports whether this run may ask the user a question. It is a
	// seam rather than a stdin type-assertion so a test can drive the prompt
	// path without a pty. nil means non-interactive, which is the safe default:
	// a run that cannot be asked is never blocked waiting for an answer.
	Interactive func() bool

	// OutputTerminal reports whether stdout is a terminal, for a command whose
	// question and whose report both go to stdout: `astro use | grep prod` has a
	// terminal on stdin and still wants the report, not a picker it cannot see.
	// nil means not a terminal.
	OutputTerminal func() bool

	// JSONStyle lays out a json result. The zero value, cliout.StyleAuto, is
	// production's: decided from Stdout, indented and colored on a terminal,
	// compact anywhere else. A test is never on a terminal, so it sets this to
	// see what a terminal gets. A stream's events are one line either way.
	JSONStyle cliout.Style

	// AirflowDefault is the Airflow `astro init` gives a project that states
	// none: the series, the requires-python its runtime ships with (empty for
	// the built-in rule), and where the answer came from. Production reads the
	// runtime catalog through runtimeversions.Default, which never fails. A
	// seam so a test never reaches the network; nil answers the built-in
	// series with no lookup.
	AirflowDefault func(ctx context.Context) (series, requiresPython string, src runtimeversions.Source)

	// RuntimeCheck holds a manifest's [tool.astro] runtime build to its Airflow
	// pin with what the runtime catalog says about it, where an image is about
	// to be built from it: a Docker-mode start, and `astro package`. Warnings
	// come back to show; a blocking finding is the error. Production reads the
	// catalog through runtimecatalog.CheckRuntime, which never fails for want
	// of it. A seam so a test never reaches the network; nil checks nothing.
	RuntimeCheck func(ctx context.Context, runtime, airflowPin string) ([]runtimeversions.Finding, error)

	// AstroBuild looks up the Astronomer build of Airflow a deployment of a
	// pin and runtime build runs, which init writes into [tool.uv] and a
	// start keeps current. Production reads the catalog and Astronomer's
	// index through runtimecatalog.AstroBuild. A seam so a test never reaches
	// the network; nil writes nothing.
	AstroBuild func(ctx context.Context, airflowPin, runtime string) (runtimeversions.AstroBuild, error)

	// RuntimeCatalog reads the runtime catalog for an edit that moves the
	// Airflow pin, which uses it to move requires-python and [tool.astro]
	// runtime with the pin. nil, or a nil result, means no catalog: the edit
	// then follows the built-in rules. Production reads it through
	// runtimecatalog.Catalog. A seam so a test never reaches the network.
	RuntimeCatalog func(ctx context.Context) *runtimeversions.Catalog

	// RuntimeConstraints reads the constraints a runtime build's image
	// installs under for one Python (runtimecatalog.RuntimeConstraints),
	// which init and a start hold [tool.uv] to beside the Airflow pins. A
	// seam so a test never reaches the network; nil writes none.
	RuntimeConstraints func(ctx context.Context, build, python string) ([]string, error)

	// RuntimeFreeze reads every package a runtime build's image has
	// installed for one Python (runtimecatalog.RuntimeFreeze), which
	// astro local check names when a Dag imports one the project does not
	// list. A seam so a test never reaches the network; nil gives no hint.
	RuntimeFreeze func(ctx context.Context, build, python string) ([]string, error)

	// ImageCatalog reads the runtime catalog for `astro package astro`'s
	// generated image: its Python, and an Airflow 2 base with no
	// [tool.astro] runtime. nil, or a nil result, means no catalog: the image
	// runs the runtime's default Python, and an Airflow 2 project with no
	// runtime named is refused. Production reads it through
	// runtimecatalog.ForBuild, which waits longer than RuntimeCatalog for an
	// Airflow 2 pin, whose base has no fallback, and as long for any other.
	ImageCatalog func(ctx context.Context, airflowPin string) *runtimeversions.Catalog

	// PackageAstro is the target `astro package astro` builds with. nil is
	// production's, which drives a container engine; a test injects a fake so
	// it can watch what the command prints around a build without one.
	PackageAstro pack.Target

	// Lock runs `uv lock` in a project, which init does after it changes
	// [tool.uv] so uv.lock follows. Production discovers uv as a standalone
	// start does (uvLock); an error wrapping uv.ErrNotFound means no uv. A
	// seam so a test runs no uv; nil locks nothing.
	Lock func(ctx context.Context, dir string) error

	// LaunchOtto starts an interactive Otto session with prompt as its first
	// message and waits for it to end, as `astro otto "<prompt>"` does. The
	// root wires it, because Otto reads config/, which this tree never
	// imports. nil means Otto cannot be started from here.
	LaunchOtto func(prompt string) error

	// Legacy1xSetting reads key, an Astro CLI 1.x setting v2 does not apply,
	// for the project at projectDir as 1.x did (its .astro/config.yaml over
	// the home config): its value when the user chose one (not 1.x's
	// default), "" otherwise; the file that sets it; and what replaces it, as
	// `astro config` says when it refuses the setting. A start reads it only
	// to say what replaces the setting (warnLegacy1xSettings). The root wires
	// it, because the read touches config/, which this tree never imports.
	// nil reads as nothing set.
	Legacy1xSetting func(projectDir, key string) (value, file, replacement string)
}

// Runtime mirrors the package-level functions of pkg/localrt as an
// interface, so commands can be tested without a real runtime.
type Runtime interface {
	Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error)
	Attach(projectPath string) (localrt.Airflow, error)
	// Stopped returns a handle for running commands in a standalone project's
	// venv while no Airflow runs on it, under the environment a start of p
	// would give Airflow. It backs `astro local run` and `shell` offline.
	Stopped(p localrt.Plan) (localrt.Airflow, error)
	// LogSource returns a handle for reading a project's logs. Unlike Attach
	// it also serves a stopped standalone project, whose log file outlives its
	// record; the handle's Logs reads that file.
	LogSource(projectPath string) (localrt.Airflow, error)
	ReadStatus(projectPath string) (localrt.Status, error)
	List() ([]localrt.Status, error)
	// PruneStale removes the records (and their routes) whose runtime is
	// gone, returning what it removed. It backs `astro local list --clean`.
	// Removing the last route also stops the proxy daemon, as a stop does.
	PruneStale() ([]localrt.Status, error)
	// Reset stops a project's Airflow if it is running and wipes the state a
	// run derives. Unlike Stop it does not go through Attach, so it serves a
	// project that is already stopped — which is the state its record is
	// removed in, and the one someone resetting a bad database is usually in.
	Reset(ctx context.Context, projectPath string) (localrt.ResetReport, error)
}

// NewDeps builds the production Deps. Call it once, from main.
func NewDeps() Deps {
	runner := checks.NewVenvRunner()
	d := Deps{
		Stdin:              os.Stdin,
		Stdout:             os.Stdout,
		Stderr:             os.Stderr,
		Checks:             runner,
		CheckVenv:          runner,
		Provisioner:        newUVProvisioner,
		WorkingDir:         os.Getwd,
		OpenURL:            browser.OpenURL,
		WorkspaceClients:   emenv.Clients,
		Session:            astrosession.BearerFor,
		Locator:            instancelocate.New,
		Interactive:        stdinIsTerminal,
		OutputTerminal:     stdoutIsTerminal,
		AirflowDefault:     catalogDefault,
		RuntimeCheck:       runtimecatalog.CheckRuntime,
		AstroBuild:         runtimecatalog.AstroBuild,
		RuntimeCatalog:     runtimecatalog.Catalog,
		RuntimeConstraints: runtimecatalog.RuntimeConstraints,
		RuntimeFreeze:      runtimecatalog.RuntimeFreeze,
		Lock:               uvLock,
		ImageCatalog: func(ctx context.Context, airflowPin string) *runtimeversions.Catalog {
			return runtimecatalog.ForBuild(airflowPin)(ctx)
		},
	}
	// One resolution of the routes dir for the whole run. Without a home the
	// runtime is a stand-in whose every call says so; commands that never
	// reach the runtime run as usual.
	if dir, err := proxydaemon.Dir(); err != nil {
		d.HomeErr = err
		d.Runtime = homelessRuntime{err: err}
	} else {
		d.ProxyDir = dir
		d.Runtime = newRuntime(dir)
	}
	return d
}

// stdinIsTerminal is the production answer to "can this run ask a question".
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// stdoutIsTerminal is the production answer to "will the user see what this
// run prints".
func stdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// skipPreRunAnnotation mirrors internal/telemetry.SkipPreRunAnnotation. It
// is spelled out here because core packages never import config/, which
// internal/telemetry pulls in. The shell root's PersistentPreRunE checks this
// annotation on the invoked command, so `astro local` stays offline: no
// network call runs before the command does. The removal stubs (`astro dev`,
// `astro run`) carry cliout.RemovedCommandAnnotation instead, for which the
// root only sets up logging and records the command where telemetry already
// holds state.
const skipPreRunAnnotation = "skipPreRun"

// markSkipPreRun annotates cmd and every descendant. Cobra annotations do
// not inherit, and the shell root reads the annotation off the leaf command.
func markSkipPreRun(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipPreRunAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markSkipPreRun(sub)
	}
}

// errAborted reports a confirmation answered "no".
var errAborted = errors.New("aborted")

// proxyDaemon adapts internal/proxydaemon's daemon lifecycle to the engines'
// localrt.ProxyDaemon seam, for the routes under dir. The engines see only the
// interface.
type proxyDaemon struct{ dir string }

func (p proxyDaemon) EnsureRunning() (string, error) {
	return proxydaemon.EnsureRunning(p.dir, proxy.DefaultPort)
}

func (p proxyDaemon) StopIfEmpty() { proxydaemon.StopIfEmpty(p.dir) }

// newProxyDaemon returns the daemon seam, or nil on Windows, where astro local
// does not start the proxy (docs/architecture.md, "Platform support"): the
// engines skip the daemon and Airflow stays reachable on its direct localhost
// port.
//
// TODO(windows-proxy): pkg/proxy's daemon now builds and runs on Windows, for
// Astro Desktop to share. Return proxyDaemon{} here too once it has been run on
// a real Windows host: started, adopted across two tools, stopped, and the
// starting terminal closed.
func newProxyDaemon(dir string) localrt.ProxyDaemon {
	if runtime.GOOS == "windows" {
		return nil
	}
	return proxyDaemon{dir: dir}
}

// newRuntime builds the shared local runtime with the CLI's composition: the
// v1-compatible routes dir (proxydaemon.Dir), the proxy daemon that shells out
// to `astro proxy`, and the real image builder.
func newRuntime(routesDir string) *localrt.Runtime {
	return localrt.New(localrt.Config{
		RoutesDir:     routesDir,
		ProxyDaemon:   newProxyDaemon(routesDir),
		Images:        newImageBuilder(),
		HealthTimeout: healthTimeout(),
		// container.binary pins the engine docker mode drives: project
		// config over global, read for the project being acted on.
		ContainerBinary: containercfg.Binary,
	})
}

// proxyDir is ProxyDir, or why there is none: the resolution's own error, or,
// for a Deps built without NewDeps, that nothing resolved one.
func (d Deps) proxyDir() (string, error) {
	switch {
	case d.ProxyDir != "":
		return d.ProxyDir, nil
	case d.HomeErr != nil:
		return "", d.HomeErr
	}
	return "", errors.New("no directory for local Airflow's proxy routes was resolved")
}

// homelessRuntime stands in for the runtime when there is no home directory
// for the proxy routes: every call returns why, so a command that needs the
// runtime fails with it and one that never asks runs as usual.
type homelessRuntime struct{ err error }

func (r homelessRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, r.err
}
func (r homelessRuntime) Attach(string) (localrt.Airflow, error)        { return nil, r.err }
func (r homelessRuntime) Stopped(localrt.Plan) (localrt.Airflow, error) { return nil, r.err }
func (r homelessRuntime) LogSource(string) (localrt.Airflow, error)     { return nil, r.err }
func (r homelessRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{}, r.err
}
func (r homelessRuntime) List() ([]localrt.Status, error)       { return nil, r.err }
func (r homelessRuntime) PruneStale() ([]localrt.Status, error) { return nil, r.err }
func (r homelessRuntime) Reset(context.Context, string) (localrt.ResetReport, error) {
	return localrt.ResetReport{}, r.err
}

// healthTimeoutEnv names how long a start waits for Airflow before giving up.
const healthTimeoutEnv = "ASTRO_LOCAL_HEALTH_TIMEOUT"

// adviseHealthTimeout adds the name of the variable that lengthens the wait to
// an error that says the wait ran out.
//
// Here rather than in the engines, which used to carry the name in their own
// message text. pkg/localrt and pkg/airflowrt are also what Astro Desktop is
// built on, and desktop fills Config.HealthTimeout from its own settings and
// never looks at this variable — so advice that is useful here was false
// there, told to a user with no shell in sight. The modules report that the
// wait ran out; naming the knob belongs to whoever owns one.
//
// Matching on the sentinel rather than on words, so the engines stay free to
// word it for their own mode: the docker one has containers that keep starting
// and the standalone one does not.
func adviseHealthTimeout(err error) error {
	// A docker start whose `compose up` ran out of time too: the same value
	// bounds it, so it is the same knob.
	if !errors.Is(err, localrt.ErrHealthTimeout) && !errors.Is(err, localrt.ErrStartTimedOut) {
		return err
	}
	return fmt.Errorf("%w; set %s (e.g. 10m) to wait longer", err, healthTimeoutEnv)
}

// adviseDatabaseNewer names the ways out of a metadata database the project's
// Airflow cannot run on, because a newer Airflow already upgraded it.
//
// Here for the reason adviseHealthTimeout is: the engines are shared with
// Astro Desktop, which resets a project from its own UI, so the command belongs
// to the layer that has one. Reset rather than a volume by name because it is
// the one command that clears the database in either mode, whichever mode the
// project was last started in.
//
// Both places a version can come from are named, because the pin is not always
// the answer: a project that declares a Dockerfile runs whatever its base image
// is. A start refuses a FROM of another series than the pin
// (scaffold.CheckDockerfileAirflow), but the build within that series, and a
// base it cannot read, are the file's alone. Advice naming only the pin sends
// that user to edit a file that changes nothing.
func adviseDatabaseNewer(err error) error {
	if !errors.Is(err, localrt.ErrDatabaseNewerThanAirflow) {
		return err
	}
	return fmt.Errorf("%w. To start over with an empty database, run astro local reset; "+
		"it deletes the Dag runs, connections and variables stored there. "+
		"To keep them, go back to the newer Airflow: the apache-airflow requirement in pyproject.toml, "+
		"or the base image in your Dockerfile if the project declares one", err)
}

// adviseStart adds what the CLI can say about a failed start to the engine's
// error. Both start paths return through it, so a new piece of advice cannot
// reach `local start` and miss `local restart`.
func adviseStart(err error) error {
	return adviseDatabaseNewer(adviseHealthTimeout(err))
}

// healthTimeout reads that variable, and returns zero for "use the default".
//
// An environment variable rather than a flag, because the two commands that
// wait — `local start` in either mode — would each need one, and the answer is
// a property of the machine rather than of the run: a slow link or a cold
// image pull is true of every start on that box, not of the one being typed.
//
// Unparseable is ignored rather than refused — this is a tuning knob with a
// working default, and a start is a bad place to learn that a variable
// somebody exported months ago has a typo in it — but it is not ignored
// silently.
//
// "600" is the spelling to expect, meaning ten minutes, because that is what
// apc.dial_timeout takes and what a reader assumes. ParseDuration refuses
// it for want of a unit, and without a word here the start runs its default
// five minutes and then advises setting the very variable that was set. The
// warning is the only thing that tells "unset" from "rejected" apart.
func healthTimeout() time.Duration {
	raw := os.Getenv(healthTimeoutEnv)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		cliout.Warn(os.Stderr, fmt.Sprintf("ignoring %s=%q, which is not a positive duration — use a unit, like 10m or 90s", healthTimeoutEnv, raw))
		return 0
	}
	return d
}

// imageBuilder adapts pkg/imagebuild to localrt.ImageBuilder. The seam exists
// because imagebuild imports the localrt contract, so localrt cannot import it
// back; this is the field-for-field copy that costs.
type imageBuilder struct {
	// catalog is how the Astro Runtime version lookup reads the runtime
	// catalog: where its copy is cached, and the User-Agent it sends. The CLI
	// names both, because the CLI is what knows where its cache lives and what
	// it is called.
	catalog runtimeversions.Options
}

// newImageBuilder reads the catalog with its default timeout: an Airflow 2
// image lookup blocks a start and has no fallback, so it waits longer than
// init's does.
func newImageBuilder() imageBuilder {
	return imageBuilder{catalog: runtimecatalog.Options(0)}
}

// Request picks the build through imagebuild.ForLocalManifest, deploy's and
// package's rule (ForManifest) over a base that also takes Airflow 2, whose
// runtime is looked up in the catalog unless the manifest names the build
// ([tool.astro] runtime).
func (b imageBuilder) Request(ctx context.Context, m imagebuild.ManifestBuild) (localrt.BuildRequest, error) {
	req, err := imagebuild.ForLocalManifest(ctx, m, b.catalog)
	if err != nil {
		return localrt.BuildRequest{}, err
	}
	return localrt.BuildRequest{
		BaseImage:    req.BaseImage,
		Dockerfile:   req.Dockerfile,
		Context:      req.Context,
		ManifestDir:  req.ManifestDir,
		Dependencies: req.Dependencies,
		FindLinks:    req.FindLinks,
		Packages:     req.Packages,
	}, nil
}

// Build copies every field across, and the two that used to be missing are
// the whole of Dockerfile mode.
//
// Without them a project declaring `dockerfile = ...` reached the builder as a
// request with no Dockerfile AND no BaseImage — the engine leaves BaseImage
// empty precisely because the file declares its own FROM — so the builder had
// nothing to build, returned an empty tag, and the compose file was written
// with `image:` blank. The start then died on "services.db-migration.image
// must be a string": a compose validation error, naming no Dockerfile, for a
// feature that had never run.
//
// What a generated build installs is resolved here, from the project's lock,
// with the host's uv (imageinstall.Fill), as a deploy and a package resolve
// it. Its warnings ride the build's progress stream, ahead of the build's own
// output.
func (imageBuilder) Build(ctx context.Context, req localrt.BuildRequest, cb localrt.Callbacks) (string, error) {
	breq := imagebuild.Request{
		WorkDir:      req.WorkDir,
		BaseImage:    req.BaseImage,
		Dockerfile:   req.Dockerfile,
		Context:      req.Context,
		Secrets:      req.Secrets,
		Tag:          req.Tag,
		ManifestDir:  req.ManifestDir,
		Dependencies: req.Dependencies,
		FindLinks:    req.FindLinks,
		Packages:     req.Packages,
		Bin:          req.Bin,
		Env:          req.Env,
	}
	warn := func(w string) {
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{Component: "build", Time: time.Now(), Text: "warning: " + w})
		}
	}
	if err := imageinstall.Fill(ctx, &breq, warn); err != nil {
		return "", err
	}
	return imagebuild.New(imagebuild.NewExecCommander(), time.Now).Build(ctx, breq, cb)
}

// ErrAllDeploymentsLinked reports a Deployment picker with nothing to offer
// because the project already links every Deployment in the workspace.
var ErrAllDeploymentsLinked = errors.New("every Deployment in the workspace is already linked")

// APCLinking is what `astro link add` needs to link an Astro Private Cloud
// Deployment, wired by the root over the Houston client and the current
// context, which this tree cannot import.
type APCLinking struct {
	// Domain is the current context's domain, the APC installation an apc
	// link is written on.
	Domain string
	// CurrentWorkspace is the context's current workspace, "" with no error
	// when none is set: the workspace a link names, and lists Deployments
	// from, without --workspace. Any other error (a context that cannot be
	// read) comes back as it is. [tool.astro] workspace never stands in for
	// it: that is an Astro workspace.
	CurrentWorkspace func() (string, error)
	// PickDeployment asks which of the workspace's Deployments to link,
	// leaving out the ones in linked (a Deployment ID to the link naming it),
	// and reports ErrAllDeploymentsLinked when that leaves none. nil means the
	// run cannot pick, and --deployment is required.
	PickDeployment func(workspaceID string, linked map[string]string) (PickedDeployment, error)
}

// ErrAPCWorkspaceUnreadable is a current APC workspace that could not be read,
// for a reason other than none being set.
var ErrAPCWorkspaceUnreadable = errors.New("reading the current Astro Private Cloud workspace")

// APCWorkspaceUnreadable says the current APC workspace could not be read, and
// that --workspace answers it, around err. An error that already says so is
// returned as it is, so the root's reader and astro link add do not repeat it.
func APCWorkspaceUnreadable(err error) error {
	if errors.Is(err, ErrAPCWorkspaceUnreadable) {
		return err
	}
	return fmt.Errorf("%w: %w; pass --workspace", ErrAPCWorkspaceUnreadable, err)
}

// PickedDeployment is the Deployment a picker chose.
type PickedDeployment struct {
	ID          string
	Name        string
	WorkspaceID string
}
