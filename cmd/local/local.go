package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/proxydaemon"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Command-name and replacement strings that appear in more than one place
// (the local tree, the dev stub's mapping, tests). Strings used once stay
// inline.
const (
	nameStart   = "start"
	nameStop    = "stop"
	nameRestart = "restart"
	nameLogs    = "logs"
	nameRun     = "run"
	nameInit    = "init"
	nameDev     = "dev"
	nameEnv     = "env"

	replaceStart   = "astro local start"
	replaceRestart = "astro local restart"
	replacePackage = "astro package"
	replaceLogs    = "astro local logs"
	replaceInit    = "astro init"

	flagWithWorkspace = "with-workspace"
	flagNoSync        = "no-sync"
)

const noSyncHelp = "With nothing running, use the project's virtualenv as it is instead of building or updating it first (also set by UV_NO_SYNC)"

const withWorkspaceHelp = "With Airflow stopped, fetch the values declared source = \"workspace\" from the Environment Manager, as a start does (needs network and astro login)"

// cli carries one command family's invocation state: the deps and the value
// of its --output flag. Built per family in the constructors below, never
// stored in a package variable.
type cli struct {
	d      Deps
	output cliout.Format
	// outage watches the astro link this run opened, if it opened one.
	outage *outageWatch
	// opened is the Airflow this run opened a client on.
	opened instances.Instance
}

func (c *cli) renderer() cliout.Renderer {
	return cliout.Renderer{Format: c.output, Out: c.d.Stdout, Err: c.d.Stderr, Style: c.d.JSONStyle}
}

// projectPath discovers the project that contains the working directory,
// walking up to find the manifest, and returns its root. Every command that
// addresses "this project" (status, logs, stop, ...) routes through here, so
// running from a subdirectory still hashes the project root, not the cwd. A
// directory outside any project surfaces *project.NotFoundError, whose
// message says what is missing.
func (c *cli) projectPath() (string, error) {
	wd, err := c.d.WorkingDir()
	if err != nil {
		return "", err
	}
	proj, err := project.Discover(wd)
	if err != nil {
		return "", err
	}
	return proj.Dir, nil
}

func (c *cli) attach() (localrt.Airflow, error) {
	dir, err := c.projectPath()
	if err != nil {
		return nil, err
	}
	return c.d.Runtime.Attach(dir)
}

// logSource returns a handle for reading logs. Unlike attach it also serves a
// stopped project, so `astro local logs` works after `astro local stop`.
func (c *cli) logSource() (localrt.Airflow, error) {
	dir, err := c.projectPath()
	if err != nil {
		return nil, err
	}
	return c.d.Runtime.LogSource(dir)
}

func (c *cli) readStatus() (localrt.Status, error) {
	dir, err := c.projectPath()
	if err != nil {
		return localrt.Status{}, err
	}
	return c.d.Runtime.ReadStatus(dir)
}

// NewLocalCmd builds the `astro local` tree. It works offline with no
// account: every command in it carries the skip-pre-run annotation.
func NewLocalCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := &cobra.Command{
		Use:          "local",
		Short:        "Run Apache Airflow locally from your project",
		Long:         "Run and manage a local Apache Airflow for the current project. Works offline, no account needed.",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		// A bare `astro local` prints help and succeeds; an unknown subcommand
		// fails. Without a RunE cobra treats this non-runnable parent as a help
		// request and exits 0 even on a typo (it returns flag.ErrHelp before it
		// validates args), so drive both cases here.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cliout.GroupHelp(cmd, nil)
			}
			// Spellings that used to work here, or that someone reaches for
			// out of v1 habit, get named rather than refused: `init` moved up
			// to `astro init`, and `ps` was always `status`.
			if replacement, ok := devReplacementFor(args[0]); ok {
				return cliout.Usage(fmt.Errorf("unknown command %q for %q. Use %s", args[0], cmd.CommandPath(), replacement))
			}
			return cliout.UnknownSubcommand(cmd, args[0])
		},
	}
	cliout.AddOutputFlag(cmd, &c.output)
	cmd.AddCommand(
		newStartCmd(c),
		newStopCmd(c),
		newRestartCmd(c),
		newStatusCmd(c),
		newListCmd(c),
		newLogsCmd(c),
		newRunCmd(c),
		newShellCmd(c),
		newOpenCmd(c),
		newResetCmd(c),
		newCheckCmd(c),
		newUpgradeCmd(c),
		newEnvCmd(c),
		newAPICmd(c),
	)
	// The query surface again, this time fixed to the Airflow this machine is
	// running: `astro local af dags list` reads the laptop, `astro af dags list`
	// reads a deployment, and neither can ever be the other.
	cmd.AddCommand(newAfCmd(d, func() target { return machineTarget{} }))
	markSkipPreRun(cmd)
	return cmd
}

func newStartCmd(c *cli) *cobra.Command {
	var opts struct {
		port            int
		docker          bool
		stopWithSession bool
		allowMissing    bool
		watch           bool
		buildSecrets    []string
	}
	cmd := &cobra.Command{
		Use:   nameStart,
		Short: "Start local Airflow for this project",
		// The wait is the one thing about a start somebody needs to change and
		// cannot from a flag, and until now the only way to find that out was
		// to hit it. A slow link or a cold image pull outlasts five minutes
		// with nothing wrong.
		Long: "Start local Airflow for this project.\n\n" +
			"A start waits up to five minutes for Airflow to answer. Set " +
			"ASTRO_LOCAL_HEALTH_TIMEOUT to a Go duration (10m, 90s) to change that.\n\n" +
			"With --watch, start stays in the foreground and keeps Airflow in step " +
			"with edits to pyproject.toml and .env: a package added to the " +
			"dependencies is installed into the running Airflow, and any other " +
			"change Airflow only reads at start (a re-pinned package, the Airflow " +
			"version, [tool.uv], the env) restarts it. A save that leaves " +
			"pyproject.toml unreadable is reported and Airflow keeps running. " +
			"Ctrl-C stops Airflow and exits.",
		Example: "  # Start Airflow for this project\n" +
			"  astro local start\n\n" +
			"  # Keep it in step with edits to pyproject.toml and .env\n" +
			"  astro local start --watch\n\n" +
			"  # In Docker rather than the default standalone mode\n" +
			"  astro local start --docker\n\n" +
			"  # On a port of your choosing\n" +
			"  astro local start --port 8081\n\n" +
			"  # Wait up to ten minutes for Airflow, on a slow machine\n" +
			"  ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := localrt.Mode("")
			if opts.docker {
				mode = localrt.ModeDocker
			}
			built, err := c.startProject(cmd.Context(), plan.Options{
				Mode:          mode,
				RequestedPort: opts.port,
				// A watch ends with this process, and so does what it
				// started: tied to the session, Airflow stops even if the
				// process dies without reaching its own stop.
				StopWithSession: opts.stopWithSession || opts.watch,
				AllowMissing:    opts.allowMissing,
			}, opts.buildSecrets)
			if err != nil || !opts.watch {
				return err
			}
			return c.runWatch(cmd.Context(), built, opts.allowMissing, opts.buildSecrets)
		},
	}
	// Refused before anything starts, rather than after a start it would
	// then have to undo.
	cmd.PreRunE = func(*cobra.Command, []string) error {
		if opts.watch && c.d.Watch == nil {
			return errWatchUnavailable
		}
		return nil
	}
	cmd.Flags().IntVar(&opts.port, "port", 0, "Preferred API server port (0 lets the runtime pick)")
	cmd.Flags().BoolVar(&opts.docker, "docker", false, "Run Airflow in Docker instead of the default standalone mode")
	cmd.Flags().BoolVar(&opts.stopWithSession, "stop-with-session", false, "Stop Airflow when this process exits instead of leaving it running")
	cmd.Flags().BoolVar(&opts.allowMissing, "allow-missing", false, "Start even if required environment values have no source, warning about each")
	cmd.Flags().BoolVar(&opts.watch, "watch", false, "Stay in the foreground, apply edits to pyproject.toml and .env to the running Airflow, and stop it on Ctrl-C")
	addBuildSecretFlag(cmd, &opts.buildSecrets)
	return cmd
}

// errWatchUnavailable is --watch on a Deps with no Watch.
var errWatchUnavailable = errors.New("--watch is not available here")

// runWatch keeps a project that --watch just started in step with its edits
// until the command is interrupted, then stops it.
//
// A change that needs a restart goes through restartProject, the same rebuild
// of the plan and the same warnings `astro local restart` gives. It keeps the
// session tie, mode and port the start set, even when Airflow went down
// under the watch and the restart is a fresh start.
func (c *cli) runWatch(ctx context.Context, built *plan.Built, allowMissing bool, buildSecretFlag []string) error {
	if c.d.Watch == nil {
		return errWatchUnavailable
	}
	r := c.renderer()
	var port int
	if st, err := c.readStatus(); err == nil {
		port = st.Port
	}
	if r.Format == cliout.FormatText {
		fmt.Fprintf(c.d.Stderr, "Watching %s and %s for changes. Press Ctrl-C to stop Airflow and exit.\n", manifest.Marker, ".env")
	}
	err := c.d.Watch(ctx, localrt.WatchOptions{
		ProjectPath: built.Project.Dir,
		Mode:        built.Plan.Mode,
		Manifest:    built.Manifest,
		Restart: func(ctx context.Context, _ localrt.Change) (*manifest.Manifest, error) {
			started, err := c.restartProject(ctx, restartOptions{
				allowMissing:    allowMissing,
				buildSecrets:    buildSecretFlag,
				stopWithSession: true,
				port:            port,
				mode:            built.Plan.Mode,
			})
			if started == nil {
				return nil, err
			}
			// What the start was built from, after the restart's own
			// rewrite of the Astronomer build pin, not before it.
			return started.Manifest, err
		},
		OnEvent:   func(e localrt.WatchEvent) { reportWatch(r, e) },
		Callbacks: c.callbacks(r),
	})
	// Interrupted, or the watch never began: stop what --watch started. The
	// session tie would stop it once this process exits; stopping here says
	// so and waits for it. Not under ctx, which the interrupt has ended.
	if serr := c.runStop(context.WithoutCancel(ctx), localrt.StopOptions{}); err == nil {
		err = serr
	}
	return err
}

// reportWatch renders one thing a watch did. In json each is a "watch" event
// carrying the localrt kind, files, requirements and reason codes; in text,
// a line for each a person needs to see.
func reportWatch(r cliout.Renderer, w localrt.WatchEvent) {
	e := event{
		Event:   "watch",
		Watch:   string(w.Kind),
		Files:   w.Files,
		Install: w.Change.Install,
		Removed: w.Change.Removed,
		Reasons: w.Change.Reasons,
	}
	if w.Err != nil {
		e.Error = w.Err.Error()
	}
	text := watchText(w)
	//nolint:errcheck // a write failure surfaces on the command's own output
	r.EmitEvent(e, func(out io.Writer) error {
		if text == "" {
			return nil
		}
		_, werr := fmt.Fprintln(out, text)
		return werr
	})
}

// watchText is the terminal line for a watch event, "" for one a person need
// not see: a save that changed nothing, and the end of a restart, which
// prints the new status itself.
func watchText(w localrt.WatchEvent) string {
	files := strings.Join(w.Files, " and ")
	switch w.Kind {
	case localrt.WatchUnreadable:
		return fmt.Sprintf("%s does not load, so Airflow keeps running as it was: %v", manifest.Marker, w.Err)
	case localrt.WatchUnchanged:
		if len(w.Change.Removed) == 0 {
			return ""
		}
		return "no longer declared, and still installed until the next restart: " + strings.Join(w.Change.Removed, ", ")
	case localrt.WatchInstalling:
		return fmt.Sprintf("%s changed: installing %s", files, strings.Join(w.Change.Install, ", "))
	case localrt.WatchInstalled:
		return "installed " + strings.Join(w.Change.Install, ", ")
	case localrt.WatchInstallFailed:
		return fmt.Sprintf("the install failed, so Airflow keeps running as it was: %v", w.Err)
	case localrt.WatchRestarting:
		reasons := make([]string, len(w.Change.Reasons))
		for i, why := range w.Change.Reasons {
			reasons[i] = why.String()
		}
		return fmt.Sprintf("%s changed: restarting, because %s", files, strings.Join(reasons, "; "))
	case localrt.WatchRestarted:
		return ""
	case localrt.WatchRestartFailed:
		return fmt.Sprintf("the restart failed: %v. Save again once it is fixed", w.Err)
	}
	return string(w.Kind)
}

// startProject starts the project in the working directory and returns what
// it built the start from.
func (c *cli) startProject(ctx context.Context, opts plan.Options, buildSecretFlag []string) (*plan.Built, error) {
	// Without a home for the proxy routes nothing can start: say so before
	// the refresh, the catalog, the plan and the secret checks.
	if c.d.HomeErr != nil {
		return nil, c.d.HomeErr
	}
	r := c.renderer()
	wd, err := c.d.WorkingDir()
	if err != nil {
		return nil, err
	}
	// Turn on Environment Manager resolution for workspace-source env values.
	opts.WorkspaceProvider = c.workspaceProvider()
	opts.BuildSecretFlags = buildSecretFlag
	opts.PythonCatalog = c.pythonCatalog(ctx)
	if opts.Mode != localrt.ModeDocker {
		c.refreshAstroBuild(ctx, r, wd)
	}
	built, err := plan.Build(wd, opts)
	if err != nil {
		return nil, c.reportBuildError(r, err)
	}
	warnManifest(r, built.ManifestWarnings)
	warnPython(r, built.PythonNote)
	c.warnLegacy1xSettings(r, built.Plan)
	warnStandaloneOmissions(r, built.Plan)
	if err := checkBuildSecrets(r, buildSecretFlag, built.Plan); err != nil {
		return nil, err
	}
	missingSecrets, err := warnMissingBuildSecrets(r, built.Plan)
	if err != nil {
		return nil, err
	}
	warnEnvValues(r, built.EnvWarnings)
	warnWorkspaceUnread(r, built.WorkspaceNote)
	warnWithout(r, "started", built.StartedWithout)
	warnLocalFilesInImage(r, built.Plan)
	if err := c.checkRuntimeBuild(ctx, r, built.Plan); err != nil {
		return nil, err
	}
	af, err := c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return nil, adviseStart(missingSecrets.Explain(err))
	}
	st, err := af.Status()
	if err != nil {
		return nil, err
	}
	// A busy explicit --port falls back to another port rather than failing;
	// say so on stderr so the user knows the URL moved and stdout stays clean
	// for json.
	if opts.RequestedPort > 0 && st.Port != opts.RequestedPort {
		fmt.Fprintf(c.d.Stderr, "requested port %d is in use; started on %d instead\n", opts.RequestedPort, st.Port)
	}
	c.noteSchedulesOff(r, built.Plan, st.Mode)
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return nil, err
	}
	c.applyPools(ctx, r, st, built.Pools)
	// The status ends the event stream the start wrote, so it is a line of
	// it: NDJSON on a terminal too.
	return built, r.EmitEvent(st, func(w io.Writer) error {
		return renderStatus(w, st, c.d.ProxyDir)
	})
}

// useJobSchedule is the Airflow setting both engines default to False, so
// the local scheduler creates no runs of its own.
const useJobSchedule = "AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE"

const schedulesOffNote = "Schedules are off locally: Dags run only when you trigger them. " +
	"To run them on their schedules, set " + useJobSchedule + "=True in .env"

// noteSchedulesOff tells a text-mode user that DAGs will not run on their
// schedules, unless the project turned them back on. On stderr, like the port
// notice, so stdout stays the status.
func (c *cli) noteSchedulesOff(r cliout.Renderer, p localrt.Plan, mode localrt.Mode) {
	if r.Format == cliout.FormatJSON || schedulesOn(p, mode) {
		return
	}
	fmt.Fprintln(c.d.Stderr, schedulesOffNote)
}

// schedulesOn reports whether the project set use_job_schedule to true where
// the engine running it reads the setting: SecretEnv over Env in both modes,
// then, in standalone only, the shell the Airflow process inherits. Docker's
// compose file keeps the default over a shell value of the same key. True is
// what Airflow's getboolean takes as true.
func schedulesOn(p localrt.Plan, mode localrt.Mode) bool {
	v, ok := p.SecretEnv[useJobSchedule]
	if !ok {
		v, ok = p.Env[useJobSchedule]
	}
	if !ok && mode != localrt.ModeDocker {
		v, ok = os.LookupEnv(useJobSchedule)
	}
	return ok && slices.Contains([]string{"t", "true", "1"}, strings.ToLower(strings.TrimSpace(v)))
}

// runtimeKey is the manifest key a runtime-build finding is about.
const runtimeKey = "tool.astro.runtime"

// checkRuntimeBuild holds a Docker-mode start's [tool.astro] runtime build to
// the pin with the runtime catalog (Deps.RuntimeCheck), before anything
// starts: a build of another Airflow series refuses the start, and a yanked
// build, one whose exact Airflow the pin excludes, or one the check could not
// read the catalog for is a warning. Standalone installs the requirement and
// builds no image, so the build decides nothing there and is not checked; a
// declared Dockerfile never has one beside it.
func (c *cli) checkRuntimeBuild(ctx context.Context, r cliout.Renderer, p localrt.Plan) error {
	if p.Mode != localrt.ModeDocker || p.Runtime == "" || p.Dockerfile != "" || c.d.RuntimeCheck == nil {
		return nil
	}
	warnings, err := c.d.RuntimeCheck(ctx, p.Runtime, p.AirflowVersion)
	for _, w := range warnings {
		emitWarning(r, event{
			Event:  "warning",
			Text:   fmt.Sprintf("%s: %s: %s", manifest.Marker, runtimeKey, w.Message),
			Key:    runtimeKey,
			Reason: w.Message,
		})
	}
	return err
}

// warnLocalFilesInImage warns, before a Docker-mode start builds the project's
// own Dockerfile, about the per-machine files that build would copy into the
// image (scaffold.LocalFilesWarning). Standalone builds no image.
func warnLocalFilesInImage(r cliout.Renderer, p localrt.Plan) {
	if p.Mode != localrt.ModeDocker {
		return
	}
	if w := scaffold.LocalFilesWarning(p.ProjectPath, p.Dockerfile); w != "" {
		emitWarning(r, event{Event: "warning", Text: w})
	}
}

// warnManifest reports the manifest's findings that do not stop a start: a
// [tool.astro.targets.<name>] key nothing reads, most likely a misspelling of
// one that is. The key and reason ride as their own fields, as an env warning's
// do, so a json consumer need not parse the prose.
func warnManifest(r cliout.Renderer, warnings []manifest.Problem) {
	for _, w := range warnings {
		emitWarning(r, event{
			Event:  "warning",
			Text:   fmt.Sprintf("%s: %s: %s", manifest.Marker, w.Key, w.Reason),
			Key:    w.Key,
			Reason: w.Reason,
		})
	}
}

// warnStandaloneOmissions warns, at start, about everything a project declares
// that standalone mode cannot honor: its own Dockerfile, and its OS packages.
// Docker mode does both, so it says nothing there. Warnings route through the
// renderer, so json mode keeps one JSON object per line.
//
// Which omissions there are, and in what order, is Plan.StandaloneOmissions'
// answer, shared with Astro Desktop. Only the wording is decided here.
//
// Both callers reach it: startProject and restartProject. `astro local restart` has no
// --docker flag of its own, so the messages name Docker mode as the thing to run
// in rather than a flag to add to the command in hand.
func warnStandaloneOmissions(r cliout.Renderer, p localrt.Plan) {
	for _, o := range p.StandaloneOmissions() {
		emitWarning(r, event{Event: "warning", Text: omissionText(o)})
	}
}

// omissionText is the terminal wording of one standalone omission. A kind this
// build has no wording for still produces a line naming it, so a new omission
// is never silently dropped.
func omissionText(o localrt.Omission) string {
	switch o.Kind {
	case localrt.OmissionDockerfile:
		return "this project declares its own Dockerfile (" + o.Dockerfile + "); standalone mode builds no image, so nothing that file installs or copies is applied. Run in Docker mode (--docker) to build it"
	case localrt.OmissionPackages:
		return "this project declares OS packages; standalone mode cannot install them, run in Docker mode (--docker) or install them yourself"
	case localrt.OmissionComposeOverride:
		return "this project has a " + localrt.ComposeOverrideFile + "; standalone mode runs no containers, so none of its services run and nothing it sets is applied. Run in Docker mode (--docker) to use it"
	default:
		return "this project declares " + string(o.Kind) + ", which standalone mode does not apply. Run in Docker mode (--docker) to apply it"
	}
}

// warnEnvValues reports values that resolved to something other than what their
// declaration promised — a value failing its `type`, a connection of a
// different `conn_type` — without refusing to start.
//
// Warning rather than blocking, because the value may well work: `type` is
// documentation the author wrote for their own team, and refusing to start a
// project over its own annotation is a tool arguing with its user. Missing
// values are the opposite case and do block — they reach the caller as
// *MissingEnvError, and Airflow cannot run without them.
//
// Not gated on mode, unlike warnStandaloneOmissions: a value of the wrong shape
// is the wrong shape in Docker too.
func warnEnvValues(r cliout.Renderer, warnings []envschema.Violation) {
	for _, v := range warnings {
		// The section, key and reason ride as their own fields as well as in
		// the prose, so a machine consumer does not have to regex
		// sectionLabel's human strings back into a Section and break on any
		// rewording.
		emitWarning(r, event{
			Event:   "warning",
			Text:    fmt.Sprintf("%s %s: %s", sectionLabel(v.Section), v.Key, v.Reason),
			Section: string(v.Section),
			Key:     v.Key,
			Reason:  v.Reason,
		})
	}
}

// warnWithout reports each required value a start (--allow-missing) or a
// command in a stopped project was allowed past, with the cause the resolver
// found, so a Dag that fails on one later is not a mystery. doing names what
// went ahead: "started" or "running".
func warnWithout(r cliout.Renderer, doing string, missing []envresolve.Missing) {
	for _, m := range missing {
		reason := "no source on this machine"
		if m.SourceNote != "" {
			reason = m.SourceNote
		}
		emitWarning(r, event{
			Event:   "warning",
			Text:    fmt.Sprintf("%s without %s %s: %s", doing, sectionLabel(m.Section), m.Name, reason),
			Section: string(m.Section),
			Key:     m.Name,
			Reason:  reason,
		})
	}
}

// warnWorkspaceUnread reports, in one line, that the linked workspace could
// not be read (offline, logged out, access lost) and the command went on
// without its values. A value only the workspace could supply that is
// required still refuses the start, through *plan.MissingEnvError; this is for
// the rest, which Airflow simply does not get this time.
// warnPython reports a requires-python the runtime build ships no Python for:
// the venv goes ahead, and an image of the project would be refused.
func warnPython(r cliout.Renderer, note string) {
	if note == "" {
		return
	}
	emitWarning(r, event{Event: "warning", Text: manifest.Marker + ": " + requiresPythonKey + ": " + note, Key: requiresPythonKey, Reason: note})
}

// requiresPythonKey is the manifest key a Python-choice warning is about.
const requiresPythonKey = "project.requires-python"

// pythonCatalog is plan.Options.PythonCatalog: the runtime catalog, read
// through Deps.RuntimeCatalog only when a standalone plan asks for it, at its
// 3s cap, so an offline start waits seconds at most and then keeps the rule
// that needs no catalog. nil when there is no catalog reader.
func (c *cli) pythonCatalog(ctx context.Context) func() *runtimeversions.Catalog {
	if c.d.RuntimeCatalog == nil {
		return nil
	}
	return func() *runtimeversions.Catalog { return c.d.RuntimeCatalog(ctx) }
}

func warnWorkspaceUnread(r cliout.Renderer, note string) {
	if note == "" {
		return
	}
	emitWarning(r, event{Event: "warning", Text: note, Reason: note})
}

// sectionLabel names a section the way a person would, since the wire values
// are snake_case and these strings are read by one. The machine-readable form
// travels in the event's own Section field, so rewording these is safe.
func sectionLabel(s envschema.Section) string {
	switch s {
	case envschema.SectionEnvVar:
		return "env var"
	case envschema.SectionAirflowVariable:
		return "Airflow variable"
	case envschema.SectionConnection:
		return "connection"
	}
	return string(s)
}

// emitWarning is the one definition of what a warning looks like on the wire,
// in both modes. Every warning source goes through it, so the shape cannot
// drift between them: a warning event in json, and in text a "Warning:" line
// on stderr (cliout.Warn), yellow on a terminal, so it is not read as the
// command's output. A renderer with no Err writes it to Out instead.
func emitWarning(r cliout.Renderer, e event) {
	if r.Format != cliout.FormatText {
		// JSON emits the event; any other format panics in EmitEvent, as an
		// unmapped Format must (cliout.Renderer.emit).
		//nolint:errcheck // a warning write failure surfaces on the command's own output
		r.EmitEvent(e, func(io.Writer) error { return nil })
		return
	}
	w := r.Err
	if w == nil {
		w = r.Out
	}
	cliout.Warn(w, e.Text)
}

// reportBuildError renders a plan-build failure. A *plan.MissingEnvError in
// json mode emits its structured payload on stdout; every failure is returned
// so the exit code is non-zero and the runner prints the message.
func (c *cli) reportBuildError(r cliout.Renderer, err error) error {
	var missing *plan.MissingEnvError
	if errors.As(err, &missing) && r.Format == cliout.FormatJSON {
		//nolint:errcheck // the returned err is what fails the command
		r.Emit(missing.Payload(), func(io.Writer) error { return nil })
		// The payload is the JSON error object; mark it so the root's error report
		// (cliout.Execute) does not print a second, plainer one over it.
		return cliout.JSONShown(err)
	}
	return err
}

func newStopCmd(c *cli) *cobra.Command {
	var opts struct {
		force bool
		clean bool
	}
	cmd := &cobra.Command{
		Use:   nameStop,
		Short: "Stop local Airflow for this project",
		Example: "  # Stop Airflow for this project\n" +
			"  astro local stop\n\n" +
			"  # Also remove its derived runtime state\n" +
			"  astro local stop --clean",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runStop(cmd.Context(), localrt.StopOptions{Force: opts.force, Clean: opts.clean})
		},
	}
	cmd.Flags().BoolVar(&opts.force, "force", false, "Skip the graceful shutdown window")
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Also remove derived runtime state")
	return cmd
}

func (c *cli) runStop(ctx context.Context, opts localrt.StopOptions) error {
	r := c.renderer()
	af, err := c.attach()
	if errors.Is(err, localrt.ErrNotRunning) {
		if opts.Clean {
			return fmt.Errorf("%w; astro local reset removes a stopped project's derived state", err)
		}
		// Nothing to stop is the state the caller asked for, so it succeeds,
		// the way `docker compose stop` does.
		e := event{Event: "state", State: localrt.StateStopped, AlreadyStopped: true}
		return r.EmitEvent(e, func(w io.Writer) error {
			_, werr := fmt.Fprintln(w, "airflow: already stopped")
			return werr
		})
	}
	if err != nil {
		return err
	}
	if err := af.Stop(ctx, opts); err != nil {
		return err
	}
	e := event{Event: "state", State: localrt.StateStopped}
	return r.EmitEvent(e, func(w io.Writer) error {
		_, werr := fmt.Fprintln(w, "airflow: stopped")
		return werr
	})
}

func newRestartCmd(c *cli) *cobra.Command {
	var force, allowMissing bool
	var buildSecrets []string
	cmd := &cobra.Command{
		Use:   nameRestart,
		Short: "Restart local Airflow for this project",
		Example: "  # Stop and start Airflow for this project\n" +
			"  astro local restart",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runRestart(cmd.Context(), force, allowMissing, buildSecrets)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Skip the graceful shutdown window when stopping")
	cmd.Flags().BoolVar(&allowMissing, "allow-missing", false, "Start even if required environment values have no source, warning about each")
	addBuildSecretFlag(cmd, &buildSecrets)
	return cmd
}

func (c *cli) runRestart(ctx context.Context, force, allowMissing bool, buildSecretFlag []string) error {
	_, err := c.restartProject(ctx, restartOptions{force: force, allowMissing: allowMissing, buildSecrets: buildSecretFlag})
	return err
}

// restartOptions are restartProject's inputs.
type restartOptions struct {
	force, allowMissing bool
	buildSecrets        []string
	// stopWithSession ties the new start to this process whatever the record
	// says, and port is the port to ask for when no record names one. A
	// --watch sets both, so a restart after Airflow went down still stops
	// with the watch and keeps its URL.
	stopWithSession bool
	port            int
	// mode is the mode to start in when no record names one, so a docker
	// project whose record went with its Airflow restarts in docker.
	mode localrt.Mode
}

// restartProject is runRestart, returning what it built the new start from.
func (c *cli) restartProject(ctx context.Context, o restartOptions) (*plan.Built, error) {
	allowMissing, buildSecretFlag, force := o.allowMissing, o.buildSecrets, o.force
	r := c.renderer()
	wd, err := c.d.WorkingDir()
	if err != nil {
		return nil, err
	}
	st, err := c.readStatus()
	if err != nil {
		return nil, err
	}
	port := st.Port
	if port == 0 {
		port = o.port
	}
	if st.State != localrt.StateRunning {
		// Nothing is running, so there is nothing to stop: restart falls back
		// to a plain start, in the mode a leftover record names, if any.
		mode := st.Mode
		if mode == "" {
			mode = o.mode
		}
		fallback := plan.Options{Mode: mode, AllowMissing: allowMissing, StopWithSession: o.stopWithSession, RequestedPort: port}
		started, err := c.startProject(ctx, fallback, buildSecretFlag)
		return started, err
	}
	// Rebuild the plan from the manifest and env as they are now, so a
	// restart picks up edits — but keep the running mode, port, and session
	// tie. Build before stopping: a build failure leaves Airflow untouched.
	if st.Mode != localrt.ModeDocker {
		c.refreshAstroBuild(ctx, r, wd)
	}
	built, err := plan.Build(wd, plan.Options{
		Mode:             st.Mode,
		RequestedPort:    port,
		StopWithSession:  st.StopWithSession || o.stopWithSession,
		AllowMissing:     allowMissing,
		BuildSecretFlags: buildSecretFlag,
		PythonCatalog:    c.pythonCatalog(ctx),

		WorkspaceProvider: c.workspaceProvider(),
	})
	if err != nil {
		return nil, c.reportBuildError(r, err)
	}
	af, err := c.attach()
	if err != nil {
		return nil, err
	}
	warnManifest(r, built.ManifestWarnings)
	warnPython(r, built.PythonNote)
	c.warnLegacy1xSettings(r, built.Plan)
	warnStandaloneOmissions(r, built.Plan)
	if err := checkBuildSecrets(r, buildSecretFlag, built.Plan); err != nil {
		return nil, err
	}
	missingSecrets, err := warnMissingBuildSecrets(r, built.Plan)
	if err != nil {
		return nil, err
	}
	warnEnvValues(r, built.EnvWarnings)
	warnWorkspaceUnread(r, built.WorkspaceNote)
	warnWithout(r, "started", built.StartedWithout)
	warnLocalFilesInImage(r, built.Plan)
	// Before the stop, like the build: a refusal leaves Airflow running.
	if err := c.checkRuntimeBuild(ctx, r, built.Plan); err != nil {
		return nil, err
	}
	if err := af.Stop(ctx, localrt.StopOptions{Force: force}); err != nil {
		return nil, err
	}
	af, err = c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return nil, adviseStart(missingSecrets.Explain(err))
	}
	st, err = af.Status()
	if err != nil {
		return nil, err
	}
	c.noteSchedulesOff(r, built.Plan, st.Mode)
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return nil, err
	}
	c.applyPools(ctx, r, st, built.Pools)
	// The status ends the event stream the start wrote, so it is a line of
	// it: NDJSON on a terminal too.
	return built, r.EmitEvent(st, func(w io.Writer) error {
		return renderStatus(w, st, c.d.ProxyDir)
	})
}

func newStatusCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the state of this project's local Airflow",
		Example: "  # Show whether Airflow is running\n" +
			"  astro local status\n\n" +
			"  # As JSON, for a script\n" +
			"  astro local status -o json",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runStatus()
		},
	}
	cmd.Flags().Bool("watch", false, "Keep reporting state changes until interrupted (not built yet)")
	return cmd
}

func (c *cli) runStatus() error {
	r := c.renderer()
	st, err := c.readStatus()
	if err != nil {
		return err
	}
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st, c.d.ProxyDir)
	})
}

func renderStatus(w io.Writer, st localrt.Status, proxyDir string) error {
	if _, err := fmt.Fprintf(w, "project: %s\nstate: %s\n", st.ProjectPath, st.State); err != nil {
		return err
	}
	if st.State != localrt.StateRunning {
		return nil
	}
	url := primaryURL(st, proxyDir)
	if _, err := fmt.Fprintf(w, "mode: %s\npid: %d\nurl: %s\n", st.Mode, st.PID, url); err != nil {
		return err
	}
	// When the shown URL is the hostname, print the direct port too so the
	// always-reachable fallback stays discoverable.
	if direct := directURL(st); direct != url {
		if _, err := fmt.Fprintf(w, "direct: %s\n", direct); err != nil {
			return err
		}
	}
	return nil
}

// directURL is a project's always-reachable Airflow URL, bound straight to its
// backend port on localhost.
func directURL(st localrt.Status) string {
	return fmt.Sprintf("http://localhost:%d", st.Port)
}

// primaryURL is the URL to show the user: the <name>.localhost hostname the
// proxy daemon serves when it is running, else the direct localhost URL. The
// hostname needs the daemon, so it is offered only when the daemon is up and
// the record carries a hostname; otherwise (daemon down, Windows, older
// record, no home for the proxy routes: proxyDir empty) the direct URL, which
// always works, stands in.
func primaryURL(st localrt.Status, proxyDir string) string {
	if st.Hostname != "" && proxyDir != "" {
		if port := proxydaemon.BoundPort(proxyDir); port != "" {
			return fmt.Sprintf("http://%s:%s", st.Hostname, port)
		}
	}
	return directURL(st)
}

func newListCmd(c *cli) *cobra.Command {
	var opts struct {
		all   bool
		clean bool
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every local Airflow known on this machine",
		Example: "  # List the local Airflows running on this machine\n" +
			"  astro local list\n\n" +
			"  # Include the stale records too\n" +
			"  astro local list --all\n\n" +
			"  # Remove the stale records\n" +
			"  astro local list --clean",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runList(opts.all, opts.clean)
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "Include stale records whose Airflow is no longer running")
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Remove stale records whose process or compose project is gone")
	return cmd
}

func (c *cli) runList(all, clean bool) error {
	r := c.renderer()
	if clean {
		return c.runListClean(r)
	}
	statuses, err := c.d.Runtime.List()
	if err != nil {
		return err
	}
	rows := buildListRows(statuses, all, time.Now())
	return emitRows(r, rows, len(rows), newLocalList, renderListTable)
}

func (c *cli) runListClean(r cliout.Renderer) error {
	// PruneStale is best-effort, so a failure and a removal are not
	// alternatives: report both, or someone told only "Error: ..." reruns the
	// command against records it already cleaned and reads the same error as
	// having done nothing.
	// Without a home there is nothing it could have read, so there is no
	// list to show beside the error.
	if c.d.HomeErr != nil {
		return c.d.HomeErr
	}
	removed, pruneErr := c.d.Runtime.PruneStale()
	// Everything PruneStale returns is stale by definition; show it all.
	rows := buildListRows(removed, true, time.Now())
	if err := emitRows(r, rows, len(rows), newLocalRemoved, renderRemovedRows); err != nil {
		return errors.Join(err, pruneErr)
	}
	return pruneErr
}

// State labels for a list row. Stale means the record outlived its runtime.
const (
	listStateRunning = "running"
	listStateStale   = "stopped (stale)"
)

// listRow is one line of `astro local list`, the shared value text and json
// both render. Uptime is precomputed so the two renderings never diverge.
type listRow struct {
	Project   string `json:"project"`
	Hostname  string `json:"hostname,omitempty"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
	Port      int    `json:"port,omitempty"`
	URL       string `json:"url,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
}

// buildListRows turns statuses into display rows. Without all, only running
// Airflows show; with it, stale records show too. now is a parameter so the
// uptime is testable.
func buildListRows(statuses []localrt.Status, all bool, now time.Time) []listRow {
	rows := make([]listRow, 0, len(statuses))
	for i := range statuses {
		st := &statuses[i]
		running := st.State == localrt.StateRunning
		if !running && !all {
			continue
		}
		row := listRow{
			Project:  st.ProjectPath,
			Hostname: st.Hostname,
			Mode:     modeLabel(st.Mode),
			Port:     st.Port,
		}
		if running {
			row.State = listStateRunning
			row.URL = fmt.Sprintf("http://localhost:%d", st.Port)
			if !st.StartedAt.IsZero() {
				row.StartedAt = st.StartedAt.Format(time.RFC3339)
				row.Uptime = formatUptime(now.Sub(st.StartedAt))
			}
		} else {
			// A record with no live runtime is a leftover: the process or
			// compose project is gone, but the record was never cleared.
			row.State = listStateStale
		}
		rows = append(rows, row)
	}
	return rows
}

func renderListTable(w io.Writer, rows []listRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No local Airflow found.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tHOSTNAME\tMODE\tSTATE\tPORT\tUPTIME")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Project, dash(row.Hostname), row.Mode, row.State, dash(omitZero(row.Port)), dash(row.Uptime))
	}
	return tw.Flush()
}

func renderRemovedRows(w io.Writer, rows []listRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No stale local Airflow records to remove.")
		return err
	}
	if _, err := fmt.Fprintf(w, "Removed %d stale record(s):\n", len(rows)); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "  %s\n", row.Project); err != nil {
			return err
		}
	}
	return nil
}

// modeLabel renders a mode for display; an empty mode reads as standalone,
// matching the record and route contracts.
func modeLabel(m localrt.Mode) string {
	if m == "" {
		return string(localrt.ModeStandalone)
	}
	return string(m)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// formatUptime renders a duration compactly: seconds under a minute, minutes
// under an hour, hours and minutes above.
func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	}
}

func newLogsCmd(c *cli) *cobra.Command {
	var opts struct {
		follow     bool
		tail       int
		components []string
	}
	cmd := &cobra.Command{
		Use:   nameLogs,
		Short: "Show logs from this project's local Airflow",
		Example: "  # Show every log line so far\n" +
			"  astro local logs\n\n" +
			"  # Follow the scheduler's log, starting from its last 100 lines\n" +
			"  astro local logs --component scheduler --tail 100 -f",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runLogs(cmd.Context(), opts.follow, opts.tail, opts.components)
		},
	}
	cmd.Flags().BoolVarP(&opts.follow, "follow", "f", false, "Keep streaming new lines")
	cmd.Flags().IntVar(&opts.tail, "tail", 0, "Only the last N lines (0 means all)")
	cmd.Flags().StringSliceVar(&opts.components, "component", nil, "Only these components (repeatable)")
	return cmd
}

func (c *cli) runLogs(ctx context.Context, follow bool, tail int, components []string) error {
	r := c.renderer()
	af, err := c.logSource()
	if err != nil {
		return err
	}
	var emitErr error
	logErr := af.Logs(ctx, localrt.LogOptions{
		Follow:     follow,
		Components: components,
		Tail:       tail,
		OnLine: func(l localrt.LogLine) {
			if err := r.EmitEvent(logEvent(l), func(w io.Writer) error {
				return renderLogLine(w, l)
			}); err != nil && emitErr == nil {
				emitErr = err
			}
		},
	})
	if logErr != nil {
		return logErr
	}
	return emitErr
}

func newRunCmd(c *cli) *cobra.Command {
	var withWorkspace, noSync bool
	cmd := &cobra.Command{
		Use:   nameRun + " <COMMAND> [ARGS...]",
		Short: "Run a command inside this project's Airflow environment",
		Long: "Run a command inside this project's Airflow environment.\n\n" +
			"With Airflow running, the command attaches to it and shares its port and metadata database. " +
			"With nothing running, a standalone project first builds or updates its virtualenv from pyproject.toml, " +
			"as uv run does, so the command works in a fresh checkout or in CI. The first build can take a few minutes. " +
			"Its progress goes to stderr; stdout stays the command's own. " +
			"Pass --" + flagNoSync + " (or set UV_NO_SYNC) to use the virtualenv as it is.",
		Example: "  # Run the tests in the project's environment\n" +
			"  astro local run pytest\n\n" +
			"  # Run an Airflow CLI command\n" +
			"  astro local run airflow dags list\n\n" +
			"  # Use the existing virtualenv without updating it\n" +
			"  astro local run --" + flagNoSync + " pytest",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runExec(cmd.Context(), args, withWorkspace, noSync)
		},
	}
	// Everything after the first positional belongs to the wrapped
	// command, not to us.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&withWorkspace, flagWithWorkspace, false, withWorkspaceHelp)
	cmd.Flags().BoolVar(&noSync, flagNoSync, false, noSyncHelp)
	return cmd
}

// environment is the handle `run` and `shell` work through. A running Airflow
// is attached, so a command shares its port and metadata DB. With nothing
// running, a standalone project's command runs in its venv under the
// environment a start would give it, so `astro local run pytest` needs no
// Airflow. That venv is built or brought up to date first, as `uv run` does,
// unless noSync (or UV_NO_SYNC) says to use it as it is; a running Airflow is
// never synced, because that would rebuild the venv under it. Docker mode runs
// commands inside its containers, which have to be up.
//
// A stopped run stays offline unless withWorkspace asks for the values only
// the Environment Manager holds. Then it resolves them as a start does, and a
// value it cannot fetch fails the run the way it fails a start.
func (c *cli) environment(ctx context.Context, withWorkspace, noSync bool) (localrt.Airflow, error) {
	dir, err := c.projectPath()
	if err != nil {
		return nil, err
	}
	st, err := c.d.Runtime.ReadStatus(dir)
	if err != nil {
		return nil, err
	}
	switch {
	case st.State == localrt.StateRunning:
		return c.d.Runtime.Attach(dir)
	case st.Mode == localrt.ModeDocker:
		// A busy engine can miss the probe's deadline, which reads as stopped,
		// so a docker record is still attached and the exec says if it is down.
		a, err := c.d.Runtime.Attach(dir)
		if errors.Is(err, localrt.ErrNotRunning) {
			return nil, errDockerDown
		}
		return a, err
	}
	opts := plan.Options{AllowMissing: true}
	if withWorkspace {
		opts.WorkspaceProvider = c.workspaceProvider()
	}
	built, err := plan.Build(dir, opts)
	if err != nil {
		return nil, err
	}
	var local, workspace []envresolve.Missing
	for _, m := range built.StartedWithout {
		if m.Workspace {
			workspace = append(workspace, m)
		} else {
			local = append(local, m)
		}
	}
	if withWorkspace && len(workspace) > 0 {
		r := c.renderer()
		return nil, c.reportBuildError(r, &plan.MissingEnvError{
			Project: built.Project.Dir,
			Missing: workspace,
			Next:    "provide them, then run the command again — or run it without them: leave off --" + flagWithWorkspace + ".",
		})
	}
	// On stderr: stdout belongs to the command being run.
	stderr := cliout.Renderer{Format: cliout.FormatText, Out: c.d.Stderr}
	warnWorkspaceUnread(stderr, built.WorkspaceNote)
	warnWithout(stderr, "running", local)
	warnWorkspaceSkipped(stderr, workspace)
	// A docker plan has no venv to build; Stopped refuses it as before.
	skipped := noSync || uvNoSyncSet()
	if !skipped && built.Plan.Mode != localrt.ModeDocker {
		if err := c.syncEnvironment(ctx, built.Plan); err != nil {
			// Airflow may have come up since the status was read, and a sync
			// refuses to rebuild the venv under it. Attach, as the running
			// path does.
			if now, serr := c.d.Runtime.ReadStatus(dir); serr == nil && now.State == localrt.StateRunning {
				return c.d.Runtime.Attach(dir)
			}
			return nil, err
		}
	}
	af, err := c.d.Runtime.Stopped(built.Plan)
	if skipped && errors.Is(err, localrt.ErrNotRunning) {
		err = fmt.Errorf("%w; drop --%s and unset UV_NO_SYNC to let this command build it", err, flagNoSync)
	}
	return af, err
}

// warnWorkspaceSkipped reports, in one line, the required values an offline
// run went without because only the Environment Manager holds them.
func warnWorkspaceSkipped(r cliout.Renderer, missing []envresolve.Missing) {
	if len(missing) == 0 {
		return
	}
	names := make([]string, len(missing))
	for i, m := range missing {
		names[i] = sectionLabel(m.Section) + " " + m.Name
	}
	pronoun := "it"
	if len(missing) > 1 {
		pronoun = "them"
	}
	emitWarning(r, event{
		Event: "warning",
		Text:  fmt.Sprintf("running without %s: declared source = \"workspace\"; pass --%s to fetch %s", strings.Join(names, ", "), flagWithWorkspace, pronoun),
	})
}

// errDockerDown reports a docker-mode project whose containers are down, which
// `run` and `shell` execute inside. It is a not_running failure, but a record
// does exist, so it does not carry the sentinel's own text.
var errDockerDown = notRunning("this project's Docker containers are not running; start them first: astro local start --docker")

type notRunning string

func (e notRunning) Error() string { return string(e) }

func (e notRunning) Is(target error) bool { return target == localrt.ErrNotRunning }

func (c *cli) runExec(ctx context.Context, argv []string, withWorkspace, noSync bool) error {
	af, err := c.environment(ctx, withWorkspace, noSync)
	if err != nil {
		return err
	}
	err = af.Run(ctx, argv, c.stdio())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// The wrapped command failed and has already written its own output;
		// carry its exit code out so a caller sees the real status, not 1.
		return &cliout.ExitError{Code: exitErr.ExitCode()}
	}
	return err
}

func newShellCmd(c *cli) *cobra.Command {
	var withWorkspace, noSync bool
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open a shell inside this project's Airflow environment",
		Example: "  # Open a shell with the project's environment\n" +
			"  astro local shell",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runShell(cmd.Context(), withWorkspace, noSync)
		},
	}
	cmd.Flags().String("shell", "", "Shell to launch instead of your login shell (not built yet)")
	cmd.Flags().BoolVar(&withWorkspace, flagWithWorkspace, false, withWorkspaceHelp)
	cmd.Flags().BoolVar(&noSync, flagNoSync, false, noSyncHelp)
	return cmd
}

func (c *cli) runShell(ctx context.Context, withWorkspace, noSync bool) error {
	af, err := c.environment(ctx, withWorkspace, noSync)
	if err != nil {
		return err
	}
	return af.Shell(ctx, c.stdio())
}

func (c *cli) stdio() localrt.Stdio {
	return localrt.Stdio{In: c.d.Stdin, Out: c.d.Stdout, Err: c.d.Stderr}
}

func newOpenCmd(c *cli) *cobra.Command {
	var printURL bool
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open this project's Airflow UI in the browser",
		Example: "  # Open the Airflow UI\n" +
			"  astro local open\n\n" +
			"  # Print its URL instead\n" +
			"  astro local open --print",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runOpen(printURL)
		},
	}
	cmd.Flags().BoolVar(&printURL, "print", false, "Print the URL instead of opening the browser")
	return cmd
}

// urlResult is what `astro local open --print` publishes in json mode: the
// one URL, so a script does not have to parse the text line. Named rather
// than anonymous so the schema pins can hold it.
type urlResult struct {
	URL string `json:"url"`
}

func (c *cli) runOpen(printURL bool) error {
	r := c.renderer()
	st, err := c.readStatus()
	if err != nil {
		return err
	}
	if st.State != localrt.StateRunning {
		// Wrapping the sentinel, like the query commands: this is the same
		// condition a consumer branches on, and it published no kind while
		// `astro local stop` published one for the identical situation.
		return fmt.Errorf("%w: local Airflow is %s; run %s first",
			localrt.ErrNotRunning, st.State, replaceStart)
	}
	url := primaryURL(st, c.d.ProxyDir)
	if printURL {
		return r.Emit(urlResult{URL: url}, func(w io.Writer) error {
			_, werr := fmt.Fprintln(w, url)
			return werr
		})
	}
	return c.d.OpenURL(url)
}

func newResetCmd(c *cli) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Stop local Airflow and wipe its derived state",
		Long:  "Stop this project's local Airflow and remove derived runtime state (database, logs, environment). The project itself is untouched.",
		Example: "  # Wipe the local database, logs, and environment, after a confirmation prompt\n" +
			"  astro local reset\n\n" +
			"  # Without the prompt\n" +
			"  astro local reset --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runReset(cmd.Context(), yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func (c *cli) runReset(ctx context.Context, yes bool) error {
	r := c.renderer()
	// Everything that can fail without consent fails first: a directory with
	// no project, or a run with no home for the state reset would wipe, is
	// refused before it is asked to wipe anything, and the question names the
	// project it would wipe.
	dir, err := c.projectPath()
	if err != nil {
		return err
	}
	if c.d.HomeErr != nil {
		return c.d.HomeErr
	}
	// The path is cleaned where the question is built: a directory can be
	// named with escape sequences, and the question reaches the terminal as
	// written.
	if err := c.confirmUnless(yes, fmt.Sprintf("Wipe the local Airflow state (database, logs, environment) of %s?", input.Clean(dir))); err != nil {
		return err
	}
	// Runtime.Reset rather than Stop with Clean: stop goes through Attach,
	// which needs a state record, and a stop removes one — so reset refused on
	// exactly the projects it is for, the ones already stopped.
	report, err := c.d.Runtime.Reset(ctx, dir)
	if err != nil {
		return err
	}
	return r.Emit(report, func(w io.Writer) error { return renderReset(w, report) })
}

func renderReset(w io.Writer, report localrt.ResetReport) error {
	if report.Stopped {
		if _, err := fmt.Fprintln(w, "airflow: stopped"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "state: wiped"); err != nil {
		return err
	}
	if report.ComposeProject != "" {
		if _, err := fmt.Fprintf(w, "removed compose project %s and its volumes\n", report.ComposeProject); err != nil {
			return err
		}
	}
	if report.DockerUnreachable {
		// Said rather than swallowed: a docker-mode project keeps its metadata
		// database in a volume, so "wiped" would be a lie if the engine that
		// holds it never answered.
		_, err := fmt.Fprintln(w, cliout.NoteLabel(w),
			"no container engine answered, so any docker-mode volume for this project is still there — start the engine and run this again to remove it")
		return err
	}
	return nil
}
