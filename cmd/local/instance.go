package local

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// NewUseCmd builds `astro use` for the root, wired with its own deps.
func NewUseCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newUseCmd(c)
	addOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// NewInstanceCmd builds `astro instance` for the root, wired with its own deps.
func NewInstanceCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newInstanceCmd(c)
	addOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// newUseCmd builds `astro use`: the per-project pin. With a name it pins, with
// --unset it clears, and with neither it shows the whole resolution rule —
// what the environment says, what is pinned, what is running, what the
// manifest defaults to, and which of them wins right now.
func newUseCmd(c *cli) *cobra.Command {
	var unset bool
	cmd := &cobra.Command{
		Use:   "use [INSTANCE]",
		Short: "Pin the instance this project acts on, or show what it resolves to",
		Long: "Pin which Airflow this project's commands act on. The pin is per project and per user — it lives in " +
			"your cache, never in the repo, so pinning here cannot redirect another checkout.\n\n" +
			"The pin is one layer of the resolution rule: -i/--instance beats " + instances.EnvVar + ", which beats the " +
			"pin, which beats the local Airflow running for this project, which beats the manifest's default link. " +
			"`astro use local` points back at the local Airflow; `astro use --unset` clears the pin.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch {
			case unset && len(args) > 0:
				return errors.New("--unset takes no instance name: it clears the pin")
			case unset:
				return c.runUseUnset()
			case len(args) == 1:
				return c.runUse(args[0])
			default:
				return c.runUseShow()
			}
		},
	}
	cmd.Flags().BoolVar(&unset, "unset", false, "Clear the pin, so resolution falls back to what is running and the manifest default")
	return cmd
}

// newInstanceCmd builds `astro instance`: the inventory of what this project
// can act on, plus the commands that describe one of them.
func newInstanceCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "instance",
		Aliases: []string{"instances"},
		Short:   "Show the Airflows this project can act on, and describe one",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	cmd.AddCommand(append([]*cobra.Command{newInstanceListCmd(c)}, instanceQueryCmds(c)...)...)
	return cmd
}

func newInstanceListCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every instance this project can act on, and mark the current one",
		Long: "List the Airflows this project can address: every deployment link the manifest declares, plus every " +
			"local Airflow running on this machine. Nothing here is looked up over the network, so a coordinate " +
			"shows exactly as the manifest writes it.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return c.runInstanceList() },
	}
}

// useResult is what a pin write reports, the same value text and json render.
type useResult struct {
	Instance string `json:"instance,omitempty"`
	Status   string `json:"status"`
}

func (c *cli) runUse(name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, set, err := c.instanceSet()
	if err != nil {
		return err
	}
	// The reserved name always pins, whether or not an Airflow is running right
	// now: `astro use local` says "this machine from here on", and starting one
	// afterwards is the normal order.
	instance, known := set.Lookup(name)
	switch {
	case known:
		c.announceInstance(instance)
	case name == instances.LocalName:
		fmt.Fprintf(c.d.Stderr, "→ %s (nothing running yet — start it with `astro local start`)\n", name)
	default:
		return &instances.UnknownError{Layer: instances.LayerFlag, Name: name, Known: set.Names()}
	}
	if err := savePin(dir, name); err != nil {
		return err
	}
	return r.Emit(useResult{Instance: name, Status: "pinned"}, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "pinned %s for this project (astro use --unset to clear)\n", name)
		return werr
	})
}

func (c *cli) runUseUnset() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, err := c.projectPath()
	if err != nil {
		return err
	}
	if err := clearPin(dir); err != nil {
		return err
	}
	return r.Emit(useResult{Status: "unpinned"}, func(w io.Writer) error {
		_, werr := fmt.Fprintln(w, "cleared this project's instance pin")
		return werr
	})
}

// resolution is the bare `astro use` report: every sticky layer of the rule
// and the instance that wins.
type resolution struct {
	Rows   []instances.Row `json:"layers"`
	Winner string          `json:"winner,omitempty"`
	// Reason explains an empty winner — nothing declared, or several with no
	// default — so the report is useful exactly when resolution is stuck.
	Reason string `json:"reason,omitempty"`
}

func (c *cli) runUseShow() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	set, req, err := c.standing()
	if err != nil {
		return err
	}
	rows, sel, serr := set.Explain(req)
	res := resolution{Rows: rows}
	if serr != nil {
		res.Reason = serr.Error()
	} else {
		res.Winner = sel.Instance.Name
	}
	return r.Emit(res, func(w io.Writer) error { return renderResolution(w, res) })
}

func renderResolution(w io.Writer, res resolution) error {
	// The problem column earns its place only when something is wrong; an
	// always-blank column is noise in the common case.
	problems := false
	for _, row := range res.Rows {
		problems = problems || row.Problem != ""
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header("\tLAYER\tVALUE\tWHERE", "\tPROBLEM", problems))
	for _, row := range res.Rows {
		marker := ""
		if row.Wins {
			marker = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s%s\n", marker, row.Layer.Label(), dash(row.Value), dash(row.Where), column(row.Problem, problems))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if res.Winner != "" {
		_, err := fmt.Fprintf(w, "\nresolves to %s\n", res.Winner)
		return err
	}
	_, err := fmt.Fprintf(w, "\nresolves to nothing: %s\n", res.Reason)
	return err
}

// instanceRow is one line of `astro instance list`.
type instanceRow struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Where is the coordinate as written; URL is filled only when the instance
	// already knows where it answers, so a consumer can tell a resolved address
	// from a coordinate that still needs a lookup.
	Where      string `json:"where,omitempty"`
	URL        string `json:"url,omitempty"`
	Source     string `json:"source"`
	AuthMethod string `json:"auth_method,omitempty"`
	Current    bool   `json:"current"`
	// Problem is why this instance cannot be reached by name, empty when it can.
	Problem string `json:"problem,omitempty"`
}

func (c *cli) runInstanceList() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	set, req, err := c.standing()
	if err != nil {
		return err
	}
	// A set with no winner is not a failure here: listing what exists is the
	// answer to "so what can I point at?", which is exactly the question a
	// stuck resolution raises.
	current := ""
	if _, sel, serr := set.Explain(req); serr == nil {
		current = sel.Instance.Name
	}
	all := set.All()
	rows := make([]instanceRow, 0, len(all))
	for i := range all {
		rows = append(rows, instanceRow{
			Name:       all[i].Name,
			Kind:       string(all[i].Kind),
			Where:      all[i].Where,
			URL:        all[i].URL,
			Source:     string(all[i].Source),
			AuthMethod: string(all[i].Link.Auth.Method),
			Current:    all[i].Problem == "" && all[i].Name == current,
			Problem:    all[i].Problem,
		})
	}
	if len(rows) == 0 && r.Format == FormatJSON {
		// Zero instances is zero NDJSON lines, which is correct and looks
		// exactly like a crash. Say so on stderr, where it cannot corrupt the
		// stream a consumer is parsing.
		fmt.Fprintln(c.d.Stderr, emptyInstanceList)
	}
	return emitRows(r, rows, renderInstanceTable)
}

// emptyInstanceList is the one sentence both renderings use for an empty
// inventory, so text and json readers are told the same thing.
const emptyInstanceList = "No instances: this project declares no deployment links and no local Airflow is running."

func renderInstanceTable(w io.Writer, rows []instanceRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, emptyInstanceList)
		return err
	}
	problems := false
	for _, row := range rows {
		problems = problems || row.Problem != ""
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header("\tNAME\tKIND\tWHERE\tSOURCE", "\tPROBLEM", problems))
	for _, row := range rows {
		marker := ""
		if row.Current {
			marker = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s%s\n", marker, row.Name, row.Kind, dash(row.Where), row.Source, column(row.Problem, problems))
	}
	return tw.Flush()
}

// header and column add a trailing column only when it has something to say.
func header(base, extra string, show bool) string {
	if show {
		return base + extra
	}
	return base
}

func column(value string, show bool) string {
	if show {
		return "\t" + value
	}
	return ""
}

// instanceSet builds the project's instance set: the manifest's links plus
// every local Airflow alive on this machine. Liveness is why the runtime does
// the listing rather than internal/localstate directly — a leftover record is
// not a running Airflow, and resolution must not point at one.
//
// Every path is canonicalized here, at the boundary, because identity is a
// filesystem question: a record written from /private/tmp/x and a command run
// from /tmp/x are the same project (the state directory they share proves it),
// and internal/instances compares the strings it is given rather than touching
// the disk itself.
func (c *cli) instanceSet() (projectDir string, set instances.Set, err error) {
	dir, err := c.projectPath()
	if err != nil {
		return "", instances.Set{}, err
	}
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	if err != nil {
		return "", instances.Set{}, err
	}
	statuses, err := c.d.Runtime.List()
	if err != nil {
		return "", instances.Set{}, err
	}
	running := make([]instances.Local, 0, len(statuses))
	for i := range statuses {
		if statuses[i].State != localrt.StateRunning {
			continue
		}
		running = append(running, instances.Local{
			ProjectPath:  canonical(statuses[i].ProjectPath),
			Port:         statuses[i].Port,
			AirflowMajor: statuses[i].AirflowMajor,
		})
	}
	return dir, instances.Build(instances.Inputs{
		ProjectPath: canonical(dir),
		Manifest:    m,
		Running:     running,
	}), nil
}

// canonical resolves symlinks in a path so two spellings of one directory
// compare equal. A path that cannot be resolved — a project deleted while its
// Airflow runs — keeps its original spelling, which is still the best name for
// it.
func canonical(path string) string {
	resolved, err := localrt.CanonicalPath(path)
	if err != nil {
		return path
	}
	return resolved
}

// instanceRequest fills the two layers a command reads rather than parses: the
// env var and the project's pin. A command that also has flags to add sets them
// on the result.
func instanceRequest(projectDir string) (instances.Request, error) {
	req := instances.Request{Env: os.Getenv(instances.EnvVar)}
	state, err := userstate.Load(projectDir)
	var decode *userstate.DecodeError
	if errors.As(err, &decode) {
		// State that does not parse is not this command's to repair, but the
		// command that does repair it should be in the message rather than left
		// for the user to find.
		return req, fmt.Errorf("%w\nclear it with `astro use --unset`", err)
	}
	if err != nil {
		return req, err
	}
	req.Pin = state.Instance
	return req, nil
}

// standing is the front half both display commands share: the project's
// instance set, and the layers that stand between runs. Neither command parses
// a flag into the rule — they report what is true right now.
func (c *cli) standing() (instances.Set, instances.Request, error) {
	dir, set, err := c.instanceSet()
	if err != nil {
		return instances.Set{}, instances.Request{}, err
	}
	req, err := instanceRequest(dir)
	if err != nil {
		return instances.Set{}, instances.Request{}, err
	}
	return set, req, nil
}

// instanceFlags carries the two flags every Airflow-facing command takes.
type instanceFlags struct {
	instance string
	url      string
}

// addInstanceFlags registers the instance-targeting flags on cmd. One
// registration keeps the spelling identical across the whole query surface:
// -i is settled for --instance, and --url is the escape hatch for an
// Airflow no project declares.
func addInstanceFlags(cmd *cobra.Command, f *instanceFlags) {
	cmd.PersistentFlags().StringVarP(&f.instance, "instance", "i", "", "Instance to act on (a deployment link, or `local`)")
	cmd.PersistentFlags().StringVar(&f.url, "url", "", "Airflow base URL to act on directly, for an Airflow no project declares")
}

// instanceClient is the composition root for a command that talks to Airflow:
// resolve, then open a client on what resolution picked. It is the one place
// the process's seams become instance Deps, so a query command is a flag, this
// call, and a rendering.
func (c *cli) instanceClient(ctx context.Context, f instanceFlags) (instances.Selection, *airflowapi.Client, error) {
	sel, err := c.resolveInstance(f.instance, f.url)
	if err != nil {
		return instances.Selection{}, nil, err
	}
	transport, err := sel.Instance.Transport(ctx, c.instanceDeps())
	if err != nil {
		return sel, nil, err
	}
	return sel, airflowapi.New(transport), nil
}

// instanceDeps hands resolution what it needs from the process: the login and
// the coordinate lookups, the two reads that touch config/ and the cloud
// clients this tree cannot import. Everything else it asks for itself.
//
// The Google chain rides along when the lookup exposes one, so a Composer link
// resolves its URL and proves itself to the Airflow behind it through the same
// credentials. Two chains would mean a run that finds an environment it cannot
// then talk to.
func (c *cli) instanceDeps() instances.Deps {
	deps := instances.Deps{Session: c.d.Session, Locator: c.d.Locator}
	if chain, ok := c.d.Locator.(instancelocate.GoogleChain); ok {
		deps.GoogleToken, deps.GoogleAccount = chain.Google()
	}
	return deps
}

// resolveInstance is the entry point for a command that is about to act on an
// instance: it applies the rule, and when nothing selects it asks — once — and
// pins the answer, so nobody is asked twice. A run that cannot ask fails with
// the message naming all three ways to say which instance.
func (c *cli) resolveInstance(flag, url string) (instances.Selection, error) {
	if flag != "" && url != "" {
		return instances.Selection{}, instances.ErrMutuallyExclusive
	}
	// --url is the no-project escape hatch, so it is answered before anything
	// looks for a project: `astro dags list --url https://airflow.corp.dev` has
	// to work from any directory on the machine.
	if url != "" {
		sel := instances.Selection{Instance: instances.URLInstance(url), From: instances.LayerURL}
		c.announceInstance(sel.Instance)
		return sel, nil
	}
	dir, set, err := c.instanceSet()
	if err != nil {
		return instances.Selection{}, err
	}
	req, err := instanceRequest(dir)
	if err != nil {
		return instances.Selection{}, err
	}
	req.Flag = flag
	sel, err := set.Select(req)
	if err != nil {
		var ambiguous *instances.AmbiguousError
		if !errors.As(err, &ambiguous) || !c.mayPrompt() {
			return instances.Selection{}, err
		}
		if sel, err = c.pickAndPin(dir, set, ambiguous.Choices); err != nil {
			return instances.Selection{}, err
		}
	}
	c.announceInstance(sel.Instance)
	return sel, nil
}

// mayPrompt reports whether this run may ask a question: someone has to be
// there to answer, and json output has to stay a stream a program can parse —
// a prompt on stderr with the run blocked on stdin is not that.
func (c *cli) mayPrompt() bool {
	return c.interactive() && c.output != string(FormatJSON)
}

// pickAndPin asks which instance to use and writes the answer to the pin, so
// the question is asked once and never again for this project.
func (c *cli) pickAndPin(projectDir string, set instances.Set, choices []string) (instances.Selection, error) {
	name, err := c.promptForInstance(choices)
	if err != nil {
		return instances.Selection{}, err
	}
	if err := savePin(projectDir, name); err != nil {
		return instances.Selection{}, err
	}
	fmt.Fprintf(c.d.Stderr, "picked %s — pinned for this project (astro use --unset to clear)\n", name)
	return set.Select(instances.Request{Pin: name})
}

// announceInstance prints the one line every resolving command puts on stderr,
// so the target is never invisible and stdout stays clean for json. The
// parenthetical carries only what the name does not: a local Airflow and a
// --url target are already their own explanation.
func (c *cli) announceInstance(i instances.Instance) {
	switch {
	case i.Where == "" || i.Name == i.Where:
		fmt.Fprintf(c.d.Stderr, "→ %s\n", i.Name)
	case i.Kind == instances.KindLocal:
		fmt.Fprintf(c.d.Stderr, "→ %s (%s)\n", i.Name, i.Where)
	default:
		fmt.Fprintf(c.d.Stderr, "→ %s (%s %s)\n", i.Name, i.Kind, i.Where)
	}
}

// promptForInstance asks which instance to use and returns the answer. It takes
// a number or a name and nothing else: there is no default on Enter, because
// the safe answer to "which Airflow should I act on" is never one this picked
// for you. It is only ever reached on an interactive run.
func (c *cli) promptForInstance(choices []string) (string, error) {
	fmt.Fprintln(c.d.Stderr, "Which instance should this project use?")
	for i, name := range choices {
		fmt.Fprintf(c.d.Stderr, "  %d) %s\n", i+1, name)
	}
	in := bufio.NewReader(c.d.Stdin)
	for attempt := 0; attempt < promptAttempts; attempt++ {
		fmt.Fprintf(c.d.Stderr, "Choose 1-%d: ", len(choices))
		line, err := in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if err != nil && answer == "" {
			if errors.Is(err, io.EOF) {
				return "", &instances.AmbiguousError{Choices: choices}
			}
			return "", err
		}
		if name, ok := matchChoice(choices, answer); ok {
			return name, nil
		}
		fmt.Fprintf(c.d.Stderr, "Not one of the choices. ")
	}
	return "", &instances.AmbiguousError{Choices: choices}
}

// promptAttempts bounds the re-asking, so a stdin that answers but never
// answers usefully ends with the message naming the flags rather than looping.
const promptAttempts = 3

// matchChoice reads an answer as a number or a name.
func matchChoice(choices []string, answer string) (string, bool) {
	if n, err := strconv.Atoi(answer); err == nil {
		if n < 1 || n > len(choices) {
			return "", false
		}
		return choices[n-1], true
	}
	for _, name := range choices {
		if name == answer {
			return name, true
		}
	}
	return "", false
}

// interactive reports whether this run can ask a question: someone has to be at
// a terminal to answer it. A piped or redirected stdin is a script, and a
// script must decide with -i, ASTRO_INSTANCE, or a pin rather than be asked.
func (c *cli) interactive() bool {
	if c.d.Interactive != nil {
		return c.d.Interactive()
	}
	return false
}

// savePin writes the project's instance pin, leaving every other field of the
// state alone.
//
// Load-modify-write: a `astro local start` writing its port between the two
// halves loses that port. Known and accepted — both writes are a person's own
// deliberate act on their own project, seconds apart at worst, and the cost of
// locking every pin write is worse than the cost of retyping one.
func savePin(projectDir, name string) error {
	state, err := userstate.Load(projectDir)
	if err != nil {
		return err
	}
	state.Instance = name
	return userstate.Save(projectDir, state)
}

// clearPin removes the pin. It is the documented fix for a pin that resolves to
// nothing, so it also heals state it cannot parse: a file that does not decode
// would otherwise fail the one command meant to get you out of trouble, leaving
// hand-deleting a cache file as the only way back.
func clearPin(projectDir string) error {
	state, err := userstate.Load(projectDir)
	var decode *userstate.DecodeError
	if errors.As(err, &decode) {
		state = userstate.State{}
	} else if err != nil {
		return err
	}
	state.Instance = ""
	return userstate.Save(projectDir, state)
}
