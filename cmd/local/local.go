package local

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
	nameObject  = "object"
	nameDev     = "dev"

	replaceStart     = "astro local start"
	replaceStatus    = "astro local status"
	replaceLogs      = "astro local logs"
	replaceInit      = "astro init"
	replaceEnvSchema = "astro local env schema"
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

func (c *cli) projectPath() (string, error) {
	return c.d.WorkingDir()
}

func (c *cli) attach() (localrt.Airflow, error) {
	project, err := c.projectPath()
	if err != nil {
		return nil, err
	}
	return c.d.Runtime.Attach(project)
}

func (c *cli) readStatus() (localrt.Status, error) {
	project, err := c.projectPath()
	if err != nil {
		return localrt.Status{}, err
	}
	return c.d.Runtime.ReadStatus(project)
}

// NewLocalCmd builds the `astro local` tree. It works offline with no
// account: every command in it carries the skip-pre-run annotation.
func NewLocalCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := &cobra.Command{
		Use:           "local",
		Short:         "Run Apache Airflow locally from your project",
		Long:          "Run and manage a local Apache Airflow for the current project. Works offline, no account needed.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
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
		newInitCmd(c),
	)
	markSkipPreRun(cmd)
	return cmd
}

func newStartCmd(c *cli) *cobra.Command {
	var opts struct {
		port            int
		mode            string
		stopWithSession bool
	}
	cmd := &cobra.Command{
		Use:   nameStart,
		Short: "Start local Airflow for this project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runStart(cmd.Context(), opts.port, opts.mode, opts.stopWithSession)
		},
	}
	cmd.Flags().IntVar(&opts.port, "port", 0, "Preferred API server port (0 lets the runtime pick)")
	cmd.Flags().StringVar(&opts.mode, "mode", "", "How Airflow runs: standalone or docker (default: from the project manifest)")
	cmd.Flags().BoolVar(&opts.stopWithSession, "stop-with-session", false, "Stop Airflow when this process exits instead of leaving it running")
	return cmd
}

func (c *cli) runStart(ctx context.Context, port int, mode string, stopWithSession bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	project, err := c.projectPath()
	if err != nil {
		return err
	}
	m, err := parseMode(mode)
	if err != nil {
		return err
	}
	// Plan building (manifest, config and env layering) is internal/plan
	// work that lands with the engine; until then the plan carries only
	// what the command line says.
	plan := localrt.Plan{
		ProjectPath:     project,
		Mode:            m,
		StopWithSession: stopWithSession,
		RequestedPort:   port,
	}
	af, err := c.d.Runtime.Start(ctx, plan, c.callbacks(r))
	if err != nil {
		return err
	}
	st, err := af.Status()
	if err != nil {
		return err
	}
	return r.Emit(st, func(w io.Writer) error {
		return renderStatus(w, st)
	})
}

// parseMode validates a --mode value; empty means "let plan building
// decide".
func parseMode(s string) (localrt.Mode, error) {
	switch m := localrt.Mode(s); m {
	case "", localrt.ModeStandalone, localrt.ModeDocker:
		return m, nil
	default:
		return "", fmt.Errorf("unknown mode %q (supported: standalone, docker)", s)
	}
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
	cmd.Flags().BoolVar(&opts.clean, "clean", false, "Also remove derived runtime state (replaces `astro dev kill`)")
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
	af, err := c.attach()
	if err != nil {
		return err
	}
	st, err := af.Status()
	if err != nil {
		return err
	}
	if err := af.Stop(ctx, localrt.StopOptions{Force: force}); err != nil {
		return err
	}
	plan := localrt.Plan{
		ProjectPath:     st.ProjectPath,
		Mode:            st.Mode,
		StopWithSession: st.StopWithSession,
		RequestedPort:   st.Port,
	}
	af, err = c.d.Runtime.Start(ctx, plan, c.callbacks(r))
	if err != nil {
		return err
	}
	st, err = af.Status()
	if err != nil {
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
	_, err := fmt.Fprintf(w, "project: %s\nstate: %s\n", st.ProjectPath, st.State)
	if err != nil {
		return err
	}
	if st.State == localrt.StateRunning {
		_, err = fmt.Fprintf(w, "mode: %s\npid: %d\nurl: http://localhost:%d\n", st.Mode, st.PID, st.Port)
	}
	return err
}

func newListCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every local Airflow known on this machine",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runList()
		},
	}
	cmd.Flags().Bool("all", false, "Include stopped projects (not built yet)")
	return cmd
}

func (c *cli) runList() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	statuses, err := c.d.Runtime.List()
	if err != nil {
		return err
	}
	return r.Emit(statuses, func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "PROJECT\tSTATE\tMODE\tPORT")
		for _, st := range statuses {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", st.ProjectPath, st.State, st.Mode, st.Port)
		}
		return tw.Flush()
	})
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
	af, err := c.attach()
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
	return af.Run(ctx, argv, c.stdio())
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
		return fmt.Errorf("local Airflow is %s; run `%s` first", st.State, replaceStart)
	}
	url := fmt.Sprintf("http://localhost:%d", st.Port)
	if printURL {
		v := struct {
			URL string `json:"url"`
		}{url}
		return r.Emit(v, func(w io.Writer) error {
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
	if _, err := c.renderer(); err != nil {
		return err
	}
	if !yes {
		if err := c.confirm("Wipe this project's local Airflow state?"); err != nil {
			return err
		}
	}
	return c.runStop(ctx, localrt.StopOptions{Clean: true})
}

func newCheckCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate this project's DAGs without starting Airflow",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := c.renderer(); err != nil {
				return err
			}
			return notBuilt("astro local check (project validation, an earlier fix)")
		},
	}
	cmd.Flags().Bool("strict", false, "Treat warnings as errors (not built yet)")
	return cmd
}
