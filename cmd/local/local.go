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

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
)

const withWorkspaceHelp = "With Airflow stopped, fetch the values declared source = \"workspace\" from the Environment Manager, as a start does (needs network and astro login)"

// cli carries one command family's invocation state: the deps and the value
// of its --output flag. Built per family in the constructors below, never
// stored in a package variable.
type cli struct {
	d      Deps
	output string
	// outage watches the astro link this run opened, if it opened one.
	outage *outageWatch
}

func (c *cli) renderer() (Renderer, error) {
	f, err := ParseFormat(c.output)
	if err != nil {
		return Renderer{}, err
	}
	return Renderer{Format: f, Out: c.d.Stdout}, nil
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
				return cmd.Help()
			}
			// Spellings that used to work here, or that someone reaches for
			// out of v1 habit, get named rather than refused: `init` moved up
			// to `astro init`, and `ps` was always `status`.
			if replacement, ok := devReplacementFor(args[0]); ok {
				return fmt.Errorf("unknown command %q for %q. Use `%s`", args[0], cmd.CommandPath(), replacement)
			}
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	addOutputFlag(cmd, &c.output)
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
			"ASTRO_LOCAL_HEALTH_TIMEOUT to a Go duration (10m, 90s) to change that.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := localrt.Mode("")
			if opts.docker {
				mode = localrt.ModeDocker
			}
			return c.runStart(cmd.Context(), plan.Options{
				Mode:            mode,
				RequestedPort:   opts.port,
				StopWithSession: opts.stopWithSession,
				AllowMissing:    opts.allowMissing,
			}, opts.buildSecrets)
		},
	}
	cmd.Flags().IntVar(&opts.port, "port", 0, "Preferred API server port (0 lets the runtime pick)")
	cmd.Flags().BoolVar(&opts.docker, "docker", false, "Run Airflow in Docker instead of the default standalone mode")
	cmd.Flags().BoolVar(&opts.stopWithSession, "stop-with-session", false, "Stop Airflow when this process exits instead of leaving it running")
	cmd.Flags().BoolVar(&opts.allowMissing, "allow-missing", false, "Start even if required environment values have no source, warning about each")
	addBuildSecretFlag(cmd, &opts.buildSecrets)
	return cmd
}

func (c *cli) runStart(ctx context.Context, opts plan.Options, buildSecretFlag []string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	wd, err := c.d.WorkingDir()
	if err != nil {
		return err
	}
	// Turn on Environment Manager resolution for workspace-source env values.
	opts.WorkspaceProvider = c.workspaceProvider()
	opts.BuildSecrets = resolveBuildSecrets(buildSecretFlag)
	built, err := plan.Build(wd, opts)
	if err != nil {
		return c.reportBuildError(r, err)
	}
	warnManifest(r, built.ManifestWarnings)
	warnStandaloneOmissions(r, built.Plan)
	if err := checkBuildSecrets(r, buildSecretFlag, built.Plan); err != nil {
		return err
	}
	missingSecrets := warnMissingBuildSecrets(r, built.Plan)
	warnEnvValues(r, built.EnvWarnings)
	warnWithout(r, "started", built.StartedWithout)
	if err := c.checkRuntimeBuild(ctx, r, built.Plan); err != nil {
		return err
	}
	af, err := c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return adviseStart(missingSecrets.Explain(err))
	}
	st, err := af.Status()
	if err != nil {
		return err
	}
	// A busy explicit --port falls back to another port rather than failing;
	// say so on stderr so the user knows the URL moved and stdout stays clean
	// for json.
	if opts.RequestedPort > 0 && st.Port != opts.RequestedPort {
		fmt.Fprintf(c.d.Stderr, "requested port %d is in use; started on %d instead\n", opts.RequestedPort, st.Port)
	}
	c.noteSchedulesOff(r, built.Plan, st.Mode)
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return err
	}
	c.applyPools(ctx, r, st, built.Pools)
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st)
	})
}

// useJobSchedule is the Airflow setting both engines default to False, so
// the local scheduler creates no runs of its own.
const useJobSchedule = "AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE"

const schedulesOffNote = "Schedules are off locally: DAGs run only when you trigger them. " +
	"To run them on their schedules, set " + useJobSchedule + "=True in .env"

// noteSchedulesOff tells a text-mode user that DAGs will not run on their
// schedules, unless the project turned them back on. On stderr, like the port
// notice, so stdout stays the status.
func (c *cli) noteSchedulesOff(r Renderer, p localrt.Plan, mode localrt.Mode) {
	if r.Format == FormatJSON || schedulesOn(p, mode) {
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
func (c *cli) checkRuntimeBuild(ctx context.Context, r Renderer, p localrt.Plan) error {
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

// warnManifest reports the manifest's findings that do not stop a start: a
// [tool.astro.targets.<name>] key nothing reads, most likely a misspelling of
// one that is. The key and reason ride as their own fields, as an env warning's
// do, so a json consumer need not parse the prose.
func warnManifest(r Renderer, warnings []manifest.Problem) {
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
// Both callers reach it: runStart and runRestart. `astro local restart` has no
// --docker flag of its own, so the messages name Docker mode as the thing to run
// in rather than a flag to add to the command in hand.
func warnStandaloneOmissions(r Renderer, p localrt.Plan) {
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
func warnEnvValues(r Renderer, warnings []envschema.Violation) {
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
func warnWithout(r Renderer, doing string, missing []envresolve.Missing) {
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
// drift between them.
func emitWarning(r Renderer, e event) {
	//nolint:errcheck // a warning write failure surfaces on the command's own output
	r.Emit(e, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "warning: %s\n", e.Text)
		return werr
	})
}

// reportBuildError renders a plan-build failure. A *plan.MissingEnvError in
// json mode emits its structured payload on stdout; every failure is returned
// so the exit code is non-zero and the runner prints the message.
func (c *cli) reportBuildError(r Renderer, err error) error {
	var missing *plan.MissingEnvError
	if errors.As(err, &missing) && r.Format == FormatJSON {
		//nolint:errcheck // the returned err is what fails the command
		r.Emit(missing.Payload(), func(io.Writer) error { return nil })
		// The payload is the JSON error object; mark it so the shared wrapper
		// does not print a second, plainer one over it.
		return errJSONShown{err}
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runStop(cmd.Context(), localrt.StopOptions{Force: opts.force, Clean: opts.clean})
		},
	}
	cmd.Flags().BoolVar(&opts.force, "force", false, "Skip the graceful shutdown window")
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Also remove derived runtime state")
	return cmd
}

func (c *cli) runStop(ctx context.Context, opts localrt.StopOptions) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	af, err := c.attach()
	if errors.Is(err, localrt.ErrNotRunning) {
		if opts.Clean {
			return fmt.Errorf("%w; `astro local reset` removes a stopped project's derived state", err)
		}
		// Nothing to stop is the state the caller asked for, so it succeeds,
		// the way `docker compose stop` does.
		e := event{Event: "state", State: localrt.StateStopped, AlreadyStopped: true}
		return r.Emit(e, func(w io.Writer) error {
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
	return r.Emit(e, func(w io.Writer) error {
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
		Args:  cobra.NoArgs,
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
	r, err := c.renderer()
	if err != nil {
		return err
	}
	wd, err := c.d.WorkingDir()
	if err != nil {
		return err
	}
	st, err := c.readStatus()
	if err != nil {
		return err
	}
	if st.State != localrt.StateRunning {
		// Nothing is running, so there is nothing to stop: restart falls back
		// to a plain start, in the mode a leftover record names, if any.
		return c.runStart(ctx, plan.Options{Mode: st.Mode, AllowMissing: allowMissing}, buildSecretFlag)
	}
	// Rebuild the plan from the manifest and env as they are now, so a
	// restart picks up edits — but keep the running mode, port, and session
	// tie. Build before stopping: a build failure leaves Airflow untouched.
	built, err := plan.Build(wd, plan.Options{
		Mode:            st.Mode,
		RequestedPort:   st.Port,
		StopWithSession: st.StopWithSession,
		AllowMissing:    allowMissing,
		BuildSecrets:    resolveBuildSecrets(buildSecretFlag),

		WorkspaceProvider: c.workspaceProvider(),
	})
	if err != nil {
		return c.reportBuildError(r, err)
	}
	af, err := c.attach()
	if err != nil {
		return err
	}
	warnManifest(r, built.ManifestWarnings)
	warnStandaloneOmissions(r, built.Plan)
	if err := checkBuildSecrets(r, buildSecretFlag, built.Plan); err != nil {
		return err
	}
	missingSecrets := warnMissingBuildSecrets(r, built.Plan)
	warnEnvValues(r, built.EnvWarnings)
	warnWithout(r, "started", built.StartedWithout)
	// Before the stop, like the build: a refusal leaves Airflow running.
	if err := c.checkRuntimeBuild(ctx, r, built.Plan); err != nil {
		return err
	}
	if err := af.Stop(ctx, localrt.StopOptions{Force: force}); err != nil {
		return err
	}
	af, err = c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return adviseStart(missingSecrets.Explain(err))
	}
	st, err = af.Status()
	if err != nil {
		return err
	}
	c.noteSchedulesOff(r, built.Plan, st.Mode)
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return err
	}
	c.applyPools(ctx, r, st, built.Pools)
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st)
	})
}

func newStatusCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the state of this project's local Airflow",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runStatus()
		},
	}
	cmd.Flags().Bool("watch", false, "Keep reporting state changes until interrupted (not built yet)")
	return cmd
}

func (c *cli) runStatus() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	st, err := c.readStatus()
	if err != nil {
		return err
	}
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st)
	})
}

func renderStatus(w io.Writer, st localrt.Status) error {
	if _, err := fmt.Fprintf(w, "project: %s\nstate: %s\n", st.ProjectPath, st.State); err != nil {
		return err
	}
	if st.State != localrt.StateRunning {
		return nil
	}
	url := primaryURL(st)
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
// record) the direct URL, which always works, stands in.
func primaryURL(st localrt.Status) string {
	if st.Hostname != "" {
		if port := proxydaemon.BoundPort(); port != "" {
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
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runList(opts.all, opts.clean)
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "Include stale records whose Airflow is no longer running")
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Remove stale records whose process or compose project is gone")
	return cmd
}

func (c *cli) runList(all, clean bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	if clean {
		return c.runListClean(r)
	}
	statuses, err := c.d.Runtime.List()
	if err != nil {
		return err
	}
	rows := buildListRows(statuses, all, time.Now())
	return emitRows(r, rows, renderListTable)
}

func (c *cli) runListClean(r Renderer) error {
	// PruneStale is best-effort, so a failure and a removal are not
	// alternatives: report both, or someone told only "Error: ..." reruns the
	// command against records it already cleaned and reads the same error as
	// having done nothing.
	removed, pruneErr := c.d.Runtime.PruneStale()
	// Everything PruneStale returns is stale by definition; show it all.
	rows := buildListRows(removed, true, time.Now())
	if err := emitRows(r, rows, renderRemovedRows); err != nil {
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
	StartedAt string `json:"startedAt,omitempty"`
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

// emitRows renders a table of rows: one JSON object per line in json mode (so
// the output is NDJSON, not one array), the text renderer once otherwise. It is
// generic because every listing surface wants exactly this and only differs in
// what a row is.
func emitRows[T any](r Renderer, rows []T, text func(io.Writer, []T) error) error {
	if r.Format == FormatJSON {
		for _, row := range rows {
			if err := r.Emit(row, nil); err != nil {
				return err
			}
		}
		return nil
	}
	return r.Emit(rows, func(w io.Writer) error { return text(w, rows) })
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
		Args:  cobra.NoArgs,
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
	r, err := c.renderer()
	if err != nil {
		return err
	}
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
			if err := r.Emit(logEvent(l), func(w io.Writer) error {
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
	var withWorkspace bool
	cmd := &cobra.Command{
		Use:   nameRun + " [command] [args...]",
		Short: "Run a command inside this project's Airflow environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runExec(cmd.Context(), args, withWorkspace)
		},
	}
	// Everything after the first positional belongs to the wrapped
	// command, not to us.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&withWorkspace, flagWithWorkspace, false, withWorkspaceHelp)
	return cmd
}

// environment is the handle `run` and `shell` work through. A running Airflow
// is attached, so a command shares its port and metadata DB. With nothing
// running, a standalone project's command runs in its venv under the
// environment a start would give it, so `astro local run pytest` needs no
// Airflow. Docker mode runs commands inside its containers, which have to be
// up.
//
// A stopped run stays offline unless withWorkspace asks for the values only
// the Environment Manager holds. Then it resolves them as a start does, and a
// value it cannot fetch fails the run the way it fails a start.
func (c *cli) environment(withWorkspace bool) (localrt.Airflow, error) {
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
		r, err := c.renderer()
		if err != nil {
			return nil, err
		}
		return nil, c.reportBuildError(r, &plan.MissingEnvError{
			Project: built.Project.Dir,
			Missing: workspace,
			Next:    "provide them, then run the command again — or run it without them: leave off `--" + flagWithWorkspace + "`.",
		})
	}
	// On stderr: stdout belongs to the command being run.
	stderr := Renderer{Format: FormatText, Out: c.d.Stderr}
	warnWithout(stderr, "running", local)
	warnWorkspaceSkipped(stderr, workspace)
	return c.d.Runtime.Stopped(built.Plan)
}

// warnWorkspaceSkipped reports, in one line, the required values an offline
// run went without because only the Environment Manager holds them.
func warnWorkspaceSkipped(r Renderer, missing []envresolve.Missing) {
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
var errDockerDown = notRunning("this project's Docker containers are not running; start them first: `astro local start --docker`")

type notRunning string

func (e notRunning) Error() string { return string(e) }

func (e notRunning) Is(target error) bool { return target == localrt.ErrNotRunning }

func (c *cli) runExec(ctx context.Context, argv []string, withWorkspace bool) error {
	af, err := c.environment(withWorkspace)
	if err != nil {
		return err
	}
	err = af.Run(ctx, argv, c.stdio())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// The wrapped command failed and has already written its own output;
		// carry its exit code out so a caller sees the real status, not 1.
		return &ExitError{Code: exitErr.ExitCode()}
	}
	return err
}

func newShellCmd(c *cli) *cobra.Command {
	var withWorkspace bool
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open a shell inside this project's Airflow environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runShell(cmd.Context(), withWorkspace)
		},
	}
	cmd.Flags().String("shell", "", "Shell to launch instead of your login shell (not built yet)")
	cmd.Flags().BoolVar(&withWorkspace, flagWithWorkspace, false, withWorkspaceHelp)
	return cmd
}

func (c *cli) runShell(ctx context.Context, withWorkspace bool) error {
	af, err := c.environment(withWorkspace)
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
		Args:  cobra.NoArgs,
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
	r, err := c.renderer()
	if err != nil {
		return err
	}
	st, err := c.readStatus()
	if err != nil {
		return err
	}
	if st.State != localrt.StateRunning {
		// Wrapping the sentinel, like the query commands: this is the same
		// condition a consumer branches on, and it published no kind while
		// `astro local stop` published one for the identical situation.
		return fmt.Errorf("%w: local Airflow is %s; run `%s` first",
			localrt.ErrNotRunning, st.State, replaceStart)
	}
	url := primaryURL(st)
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runReset(cmd.Context(), yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func (c *cli) runReset(ctx context.Context, yes bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	if err := c.confirmUnless(yes, "Wipe this project's local Airflow state?"); err != nil {
		return err
	}
	dir, err := c.projectPath()
	if err != nil {
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
		_, err := fmt.Fprintln(w,
			"note: no container engine answered, so any docker-mode volume for this project is still there — start the engine and run this again to remove it")
		return err
	}
	return nil
}
