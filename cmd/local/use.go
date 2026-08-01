package local

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/userstate"
)

// NewUseCmd builds `astro use` for the root, wired with its own deps.
func NewUseCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newUseCmd(c)
	addOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// newUseCmd builds `astro use`: the per-project pin, and the inventory. With a
// name it pins, with --unset it clears, and with neither it shows the whole
// resolution rule — what the environment says, what is pinned, what the
// manifest defaults to, which of them wins right now, and everything this
// project could point at instead.
func newUseCmd(c *cli) *cobra.Command {
	var unset bool
	cmd := &cobra.Command{
		Use:   "use [DEPLOYMENT]",
		Short: "Pin the deployment this project acts on, or show what it resolves to",
		Long: "Pin which deployment this project's commands act on. The pin is per project and per user — it lives " +
			"in your cache, never in the repo, so pinning here cannot redirect another checkout.\n\n" +
			"The pin is one layer of the resolution rule: -d/--deployment beats " + instances.EnvVar + ", which beats " +
			"the pin, which beats the manifest's default link. `astro use --unset` clears the pin.\n\n" +
			"Deployments only. The Airflow running on this machine is not one of them and is never resolved to: " +
			"`astro local dags list`, `astro local health`, and the rest of `astro local` act on it.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch {
			case unset && len(args) > 0:
				return errors.New("--unset takes no deployment name: it clears the pin")
			case unset:
				return c.runUseUnset()
			case len(args) == 1:
				return c.runUse(args[0])
			default:
				return c.runUseShow()
			}
		},
	}
	cmd.Flags().BoolVar(&unset, "unset", false, "Clear the pin, so resolution falls back to the manifest's default link")
	return cmd
}

// useResult is what a pin write reports, the same value text and json render.
type useResult struct {
	Deployment string `json:"deployment,omitempty"`
	Status     string `json:"status"`
}

func (c *cli) runUse(name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, set, err := c.deploymentSet()
	if err != nil {
		return err
	}
	instance, known := set.Lookup(name)
	if !known {
		// LayerFlag, not LayerPin: the name was typed here and now, so the
		// refusal must not send the reader off to clear a pin they never wrote.
		return set.Unknown(instances.LayerFlag, name)
	}
	// Announce after the write, not before: the arrow says "this is what you are
	// pointed at now", and a failed write means you are not.
	if err := savePin(dir, name); err != nil {
		return pinStateError(err)
	}
	c.announceInstance(instance)
	return r.Emit(useResult{Deployment: name, Status: "pinned"}, func(w io.Writer) error {
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
		_, werr := fmt.Fprintln(w, "cleared this project's deployment pin")
		return werr
	})
}

// resolution is the bare `astro use` report: every sticky layer of the rule,
// the deployment that wins, and the inventory of what else there is. The two
// halves answer one question between them — "what am I pointed at, and what
// could I point at instead" — so they are one report rather than two commands.
type resolution struct {
	Rows   []instances.Row `json:"layers"`
	Winner string          `json:"winner,omitempty"`
	// Reason explains an empty winner — nothing linked, or several with no
	// default — so the report is useful exactly when resolution is stuck.
	Reason string `json:"reason,omitempty"`
	// Instances is everything this project can act on: its deployment links,
	// and the local Airflows running on this machine, which are reached by
	// spelling a command `astro local …` rather than by name.
	Instances []instanceRow `json:"instances"`
}

func (c *cli) runUseShow() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, set, req, err := c.standing()
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
	if res.Instances, err = c.inventory(dir, set, res.Winner); err != nil {
		return err
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
	head, problem := "\tLAYER\tVALUE\tWHERE", func(string) string { return "" }
	if problems {
		head += "\tPROBLEM"
		problem = func(value string) string { return "\t" + value }
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, head)
	for _, row := range res.Rows {
		marker := ""
		if row.Wins {
			marker = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s%s\n", marker, row.Layer.Label(), dash(row.Value), dash(row.Where), problem(row.Problem))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	verdict := fmt.Sprintf("\nresolves to %s\n\n", res.Winner)
	if res.Winner == "" {
		verdict = fmt.Sprintf("\nresolves to nothing: %s\n\n", res.Reason)
	}
	if _, err := io.WriteString(w, verdict); err != nil {
		return err
	}
	return renderInstanceTable(w, res.Instances)
}

// instanceRow is one line of the inventory: a deployment this project links, or
// a local Airflow running on this machine.
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
	// Note is how to reach a row that `astro use` cannot pin. Only deployments
	// are addressable by name, so every running Airflow carries one: a listing
	// that prints a name the very next command refuses is worse than one that
	// says up front what the name is for.
	Note string `json:"note,omitempty"`
}

// inventory is everything this project can act on: the deployments it links,
// and the local Airflows running on this machine. Nothing here is looked up
// over the network, so a coordinate shows exactly as the manifest writes it.
//
// The local rows are there to be seen, not selected. They are informational —
// `astro use` pins deployments only — so each carries the note that says how to
// reach it instead.
func (c *cli) inventory(projectDir string, set instances.Set, current string) ([]instanceRow, error) {
	links := set.All()
	rows := make([]instanceRow, 0, len(links))
	// taken is every name already spoken for. A running Airflow may not quietly
	// answer to a deployment's name, or to another machine's.
	taken := map[string]bool{instances.LocalName: true}
	for i := range links {
		taken[links[i].Name] = true
		rows = append(rows, instanceRow{
			Name:       links[i].Name,
			Kind:       string(links[i].Kind),
			Where:      links[i].Where,
			URL:        links[i].URL,
			Source:     string(links[i].Source),
			AuthMethod: string(links[i].Link.Auth.Method),
			Current:    links[i].Name == current,
		})
	}
	running, err := c.runningLocals()
	if err != nil {
		return nil, err
	}
	own := canonical(projectDir)
	for _, l := range running {
		it := instances.LocalInstance(l, localRowName(l, own, taken))
		taken[it.Name] = true
		rows = append(rows, instanceRow{
			Name:   it.Name,
			Kind:   string(it.Kind),
			Where:  it.Where,
			URL:    it.URL,
			Source: string(it.Source),
			Note:   localRowNote(it, l.ProjectPath == own),
		})
	}
	return rows, nil
}

// localRowName is what a running Airflow is called in the listing. This
// project's own is `local`, the word the whole `astro local` surface means.
// Every other one takes its directory name, unless that name is already spoken
// for — by the reserved word, by a deployment the manifest links, or by another
// checkout with the same basename — in which case it takes its path, the one
// thing about it that is unique.
//
// These names are display only, but they still have to be distinct: a listing
// with two rows called `billing` cannot be read, and a consumer keying on name
// cannot tell them apart.
func localRowName(l instances.Local, own string, taken map[string]bool) string {
	if l.ProjectPath == own {
		return instances.LocalName
	}
	if base := filepath.Base(l.ProjectPath); !taken[base] {
		return base
	}
	return l.ProjectPath
}

// localRowNote says how to reach a row that is listed but cannot be pinned.
// This machine's own Airflow has a whole command surface of its own; anything
// else running is reachable only as a bare URL, because no project in front of
// the user declares it.
func localRowNote(i instances.Instance, own bool) string {
	switch {
	case own:
		return "this machine — `astro local dags list`, `astro local health`, …"
	case i.URL == "":
		// The record carried no port, so there is no address to offer at all.
		return "another project's Airflow; its record has no port"
	default:
		return "another project's Airflow — reach it with --url " + i.URL
	}
}

// emptyInventory is what the listing prints for a project with nothing to act
// on, where an empty table would read as a bug.
const emptyInventory = "Nothing to act on: this project links no deployments and no local Airflow is running."

func renderInstanceTable(w io.Writer, rows []instanceRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, emptyInventory)
		return err
	}
	// The note column earns its place only when a row is not addressable, which
	// is exactly when a reader needs telling.
	notes := false
	for _, row := range rows {
		notes = notes || row.Note != ""
	}
	head, note := "\tNAME\tKIND\tWHERE\tSOURCE", func(string) string { return "" }
	if notes {
		head += "\tNOTE"
		note = func(value string) string { return "\t" + value }
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, head)
	for _, row := range rows {
		marker := ""
		if row.Current {
			marker = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s%s\n", marker, row.Name, row.Kind, dash(row.Where), row.Source, note(row.Note))
	}
	return tw.Flush()
}

// savePin writes the project's deployment pin, leaving every other field of the
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

// pinStateError carries the way out when state that will not parse blocks a
// command. Reading the pin hits it, and so does writing one — `astro use <name>`
// loads the file before it saves it — so both paths name the same fix rather
// than one of them surfacing a bare decode error.
func pinStateError(err error) error {
	var decode *userstate.DecodeError
	if errors.As(err, &decode) {
		return fmt.Errorf("%w\nclear it with `astro use --unset`", err)
	}
	return err
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
