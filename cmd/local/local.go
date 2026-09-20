package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
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

	replaceStart = "astro local start"
	replaceLogs  = "astro local logs"
	replaceInit  = "astro init"
)

// cli carries one command family's invocation state: the deps and the value
// of its --output flag. Built per family in the constructors below, never
// stored in a package variable.
type cli struct {
	d      Deps
	output string
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
			})
		},
	}
	cmd.Flags().IntVar(&opts.port, "port", 0, "Preferred API server port (0 lets the runtime pick)")
	cmd.Flags().BoolVar(&opts.docker, "docker", false, "Run Airflow in Docker instead of the default standalone mode")
	cmd.Flags().BoolVar(&opts.stopWithSession, "stop-with-session", false, "Stop Airflow when this process exits instead of leaving it running")
	return cmd
}

func (c *cli) runStart(ctx context.Context, opts plan.Options) error {
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
	built, err := plan.Build(wd, opts)
	if err != nil {
		return c.reportBuildError(r, err)
	}
	warnStandaloneOmissions(r, built.Plan)
	warnEnvValues(r, built.EnvWarnings)
	af, err := c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return adviseHealthTimeout(err)
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
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return err
	}
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st)
	})
}

// warnStandaloneOmissions warns, at start, about everything a project declares
// that standalone mode cannot honor: its own Dockerfile, and its OS packages.
// Docker mode does both, so it says nothing there. Warnings route through the
// renderer, so json mode keeps one JSON object per line.
//
// Named for what it does rather than for one of the two keys. It was
// warnStandalonePackages when packages were the only case, and the name outlived
// that by exactly one commit — the reader checking whether the Dockerfile case is
// handled would have found a function and a comment both saying it is not.
//
// Both callers reach it: runStart and runRestart. `astro local restart` has no
// --docker flag of its own, so the messages name Docker mode as the thing to run
// in rather than a flag to add to the command in hand.
func warnStandaloneOmissions(r Renderer, p localrt.Plan) {
	if p.Mode == localrt.ModeDocker {
		return
	}
	for _, text := range standaloneOmissions(p) {
		emitWarning(r, event{Event: "warning", Text: text})
	}
}

// standaloneOmissions lists what this plan declares that standalone mode cannot
// honor, in the order it matters.
//
// The Dockerfile warning is the one that was missing, and it is the larger of
// the two by a distance: OS packages are a line in the manifest, while a declared
// Dockerfile IS the build. `astro local start` on a project whose Dockerfile does
// `RUN apt-get install -y unixodbc-dev` and `COPY vendored/ /opt/vendored`
// provisions an environment with none of it — and standalone is the DEFAULT, so
// this is what a user gets by typing the shortest command. rt.Plan's own doc says
// standalone ignores the field; nothing said it to the person it happens to.
//
// Both, and in this order, because a project can declare both and the Dockerfile
// is the one that changes what they should do about it.
func standaloneOmissions(p localrt.Plan) []string {
	var out []string
	if p.Dockerfile != "" {
		out = append(out, "this project declares its own Dockerfile ("+p.Dockerfile+"); standalone mode builds no image, so nothing that file installs or copies is applied. Run in Docker mode (--docker) to build it")
	}
	if len(p.Packages) > 0 {
		out = append(out, "this project declares OS packages; standalone mode cannot install them, run in Docker mode (--docker) or install them yourself")
	}
	return out
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
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Also remove derived runtime state (replaces the old astro dev kill)")
	return cmd
}

func (c *cli) runStop(ctx context.Context, opts localrt.StopOptions) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	af, err := c.attach()
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
	var force bool
	cmd := &cobra.Command{
		Use:   nameRestart,
		Short: "Restart local Airflow for this project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runRestart(cmd.Context(), force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Skip the graceful shutdown window when stopping")
	return cmd
}

func (c *cli) runRestart(ctx context.Context, force bool) error {
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
		// Nothing is running (the record is gone), so there is nothing to stop
		// and no prior mode or port to carry: restart falls back to a plain
		// start with the defaults.
		return c.runStart(ctx, plan.Options{})
	}
	// Rebuild the plan from the manifest and env as they are now, so a
	// restart picks up edits — but keep the running mode, port, and session
	// tie. Build before stopping: a build failure leaves Airflow untouched.
	built, err := plan.Build(wd, plan.Options{
		Mode:            st.Mode,
		RequestedPort:   st.Port,
		StopWithSession: st.StopWithSession,

		WorkspaceProvider: c.workspaceProvider(),
	})
	if err != nil {
		return c.reportBuildError(r, err)
	}
	af, err := c.attach()
	if err != nil {
		return err
	}
	warnStandaloneOmissions(r, built.Plan)
	warnEnvValues(r, built.EnvWarnings)
	if err := af.Stop(ctx, localrt.StopOptions{Force: force}); err != nil {
		return err
	}
	af, err = c.d.Runtime.Start(ctx, built.Plan, c.callbacks(r))
	if err != nil {
		return adviseHealthTimeout(err)
	}
	st, err = af.Status()
	if err != nil {
		return err
	}
	if err := plan.PersistPort(built.Project.Dir, st.Port); err != nil {
		return err
	}
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
	cmd := &cobra.Command{
		Use:   nameRun + " [command] [args...]",
		Short: "Run a command inside this project's Airflow environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runExec(cmd.Context(), args)
		},
	}
	// Everything after the first positional belongs to the wrapped
	// command, not to us.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().Bool("no-wait", false, "Do not wait for Airflow to be healthy first (not built yet)")
	return cmd
}

func (c *cli) runExec(ctx context.Context, argv []string) error {
	af, err := c.attach()
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
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open a shell inside this project's Airflow environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runShell(cmd.Context())
		},
	}
	cmd.Flags().String("shell", "", "Shell to launch instead of your login shell (not built yet)")
	return cmd
}

func (c *cli) runShell(ctx context.Context) error {
	af, err := c.attach()
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
