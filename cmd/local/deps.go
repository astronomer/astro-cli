// Package local is the v2 command tree: `astro local *`, `astro init`, the
// root aliases `astro start/stop/logs`, and the `astro dev` removal stub.
// It follows the cmd/ layer rules in docs/v2-architecture.md: parse flags,
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
	"path/filepath"
	"runtime"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Deps is everything the v2 commands need from the process. The composition
// root (the v1 root, cmd/root.go) builds it once through NewDeps and hands it
// down.
type Deps struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Runtime is the pkg/localrt surface the commands call.
	Runtime Runtime

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

	// AstroV1Client is the v1 API client the Environment Manager read-through
	// provider reads workspace-source env values through. It authenticates
	// from the current login context per request; a logged-out user just makes
	// the provider absent. A test injects a fake.
	AstroV1Client astrov1.APIClient

	// WorkspaceClients builds the client the Environment Manager read-through
	// provider reads workspace-source values with, from the stored login for
	// the manifest's domain — not the current context, so a production-linked
	// project keeps reading production while the CLI is switched to dev. Nil
	// turns workspace resolution off. A test injects a fake.
	WorkspaceClients emenv.ClientFactory

	// Session hands instance resolution the current login's bearer, for a link
	// that proves itself with the astro auth method. It is a seam because the
	// read touches config/, which this tree never imports directly; nil reads
	// as logged out.
	Session func(ctx context.Context) (string, error)

	// Locator turns a coordinate link into an Airflow base URL: a Deployment's
	// web server from the control plane, a Composer environment's Airflow URI
	// from the Composer API. It is a seam for the same reason Session is — the
	// lookups touch config/ and the cloud clients — and nil means a coordinate
	// link cannot be reached, which is what a test that declares none wants.
	Locator instances.Locator

	// LoginDomain is the Astro host the current login is for, which linking a
	// workspace writes as its domain when none is given. A seam for the same
	// reason Session is; nil reads as no login.
	LoginDomain func() (string, error)

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

	// Interactive reports whether this run may ask the user a question. It is a
	// seam rather than a stdin type-assertion so a test can drive the prompt
	// path without a pty. nil means non-interactive, which is the safe default:
	// a run that cannot be asked is never blocked waiting for an answer.
	Interactive func() bool
}

// Runtime mirrors the package-level functions of pkg/localrt as an
// interface, so commands can be tested without a real runtime.
type Runtime interface {
	Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error)
	Attach(projectPath string) (localrt.Airflow, error)
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
	astroV1Client := astrov1.NewV1Client(httputil.NewHTTPClient())
	return Deps{
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		Runtime:          newRuntime(),
		Checks:           runner,
		CheckVenv:        runner,
		Provisioner:      newUVProvisioner,
		WorkingDir:       os.Getwd,
		OpenURL:          browser.OpenURL,
		AstroV1Client:    astroV1Client,
		WorkspaceClients: emenv.Clients,
		Session:          astrosession.Bearer,
		Locator:          instancelocate.New(astroV1Client),
		LoginDomain:      astrosession.Domain,
		Interactive:      stdinIsTerminal,
	}
}

// stdinIsTerminal is the production answer to "can this run ask a question".
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// skipPreRunAnnotation mirrors internal/telemetry.SkipPreRunAnnotation. It
// is spelled out here because v2 packages never import config/, which
// internal/telemetry pulls in. The v1 root's PersistentPreRunE checks this
// annotation on the invoked command, so `astro local` stays offline: no
// network call runs before the command does.
const skipPreRunAnnotation = "skipPreRun"

// markSkipPreRun annotates cmd and every descendant. Cobra annotations do
// not inherit, and the v1 root reads the annotation off the leaf command.
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

// proxyDaemon adapts airflow/proxy's daemon lifecycle to the engines'
// localrt.ProxyDaemon seam. It lives at the composition layer because
// airflow/proxy pulls in config, which the v2 engines must not import; they
// see only the interface.
type proxyDaemon struct{}

func (proxyDaemon) EnsureRunning() (string, error) {
	return proxydaemon.EnsureRunning(proxy.DefaultPort)
}

func (proxyDaemon) StopIfEmpty() { proxydaemon.StopIfEmpty() }

// newProxyDaemon returns the daemon seam, or nil on Windows, where the proxy
// is unsupported (decision 12): the engines skip the daemon and Airflow stays
// reachable on its direct localhost port.
func newProxyDaemon() localrt.ProxyDaemon {
	if runtime.GOOS == "windows" {
		return nil
	}
	return proxyDaemon{}
}

// routesDir is where pkg/proxy keeps routes.json: <astro home>/proxy, the
// same location v1 uses, honoring the same ASTRO_HOME override — v1 and v2
// must see each other's routes.
func routesDir() string {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home, _ = os.UserHomeDir() //nolint:errcheck // falls back to a relative path, matching v1's ASTRO_HOME handling
	}
	return filepath.Join(home, ".astro", "proxy")
}

// newRuntime builds the shared local runtime with the CLI's composition: the
// v1-compatible routes dir, the proxy daemon that shells out to `astro proxy`, and
// the real image builder.
func newRuntime() *localrt.Runtime {
	return localrt.New(localrt.Config{
		RoutesDir:     routesDir(),
		ProxyDaemon:   newProxyDaemon(),
		Images:        newImageBuilder(),
		HealthTimeout: healthTimeout(),
	})
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
	if !errors.Is(err, localrt.ErrHealthTimeout) {
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
// is, whatever the pin says (see localdocker's planMajor). Advice naming only
// the pin sends that user to edit a file that changes nothing.
func adviseDatabaseNewer(err error) error {
	if !errors.Is(err, localrt.ErrDatabaseNewerThanAirflow) {
		return err
	}
	return fmt.Errorf("%w. To start over with an empty database, run `astro local reset`; "+
		"it deletes the Dag runs, connections and variables stored there. "+
		"To keep them, go back to the newer Airflow: the `airflow` pin in pyproject.toml, "+
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
// houston.dial_timeout takes and what a reader assumes. ParseDuration refuses
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
		fmt.Fprintf(os.Stderr,
			"warning: ignoring %s=%q, which is not a positive duration — use a unit, like 10m or 90s\n",
			healthTimeoutEnv, raw)
		return 0
	}
	return d
}

// imageBuilder adapts pkg/imagebuild to localrt.ImageBuilder. The seam exists
// because imagebuild imports the localrt contract, so localrt cannot import it
// back; this is the field-for-field copy that costs.
type imageBuilder struct {
	// versionCacheDir is where the Astro Runtime version lookup keeps the
	// service's answer. The CLI names it, because the CLI is what knows where
	// its cache lives.
	versionCacheDir string
}

// newImageBuilder resolves the cache directory for the version lookup. A cache
// root that cannot be resolved is not a failure: an empty directory means the
// lookup asks the service every time, which still works.
func newImageBuilder() imageBuilder {
	dir, err := localrt.CacheRoot()
	if err != nil {
		dir = ""
	}
	return imageBuilder{versionCacheDir: dir}
}

// RuntimeImage resolves through LocalRuntimeImage, not RuntimeImage: a local run
// takes either generation, and an Airflow 2 pin needs the version service to say
// which runtime carries it. The deploy path keeps the pure RuntimeImage, which
// is Airflow 3 alone.
func (b imageBuilder) RuntimeImage(ctx context.Context, airflowVersion string) (string, error) {
	return imagebuild.LocalRuntimeImage(ctx, airflowVersion, b.versionCacheDir)
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
func (imageBuilder) Build(ctx context.Context, req localrt.BuildRequest, cb localrt.Callbacks) (string, error) {
	return imagebuild.New(imagebuild.NewExecCommander(), time.Now).Build(ctx, imagebuild.Request{
		WorkDir:      req.WorkDir,
		BaseImage:    req.BaseImage,
		Dockerfile:   req.Dockerfile,
		Context:      req.Context,
		Tag:          req.Tag,
		Dependencies: req.Dependencies,
		Packages:     req.Packages,
		Bin:          req.Bin,
		Env:          req.Env,
	}, cb)
}

// ErrAllDeploymentsLinked reports a Deployment picker with nothing to offer
// because the project already links every Deployment in the workspace.
var ErrAllDeploymentsLinked = errors.New("every Deployment in the workspace is already linked")

// PickedDeployment is the Astro Deployment a picker chose.
type PickedDeployment struct {
	ID          string
	Name        string
	WorkspaceID string
}
