package local

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// NewUseCmd builds `astro use` for the root, wired with its own deps.
func NewUseCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newUseCmd(c)
	cliout.AddOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// newUseCmd builds `astro use`: your per-project selection of a linked
// Deployment. With a name it selects, and with --unset it clears. With
// neither, a person at a terminal gets a picker of the links; everyone else —
// a pipe, a script, --output json — gets the same links as a list, with the
// current one marked.
func newUseCmd(c *cli) *cobra.Command {
	var unset bool
	cmd := &cobra.Command{
		Use:   "use [DEPLOYMENT]",
		Short: "Select the Deployment this project's commands act on",
		Long: "Select which linked Deployment this project's commands act on. Your selection is saved on your\n" +
			"machine, not in the repo, so it affects only you.\n\n" +
			"Commands pick a Deployment in this order: -d/--deployment, " + instances.EnvVar + ", your selection,\n" +
			"then the project's default link.",
		Example: "  astro use            # pick from the linked Deployments\n" +
			"  astro use prod       # select prod\n" +
			"  astro use --unset    # clear your selection\n" +
			"  astro use -o json    # show what this project resolves to, and why",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch {
			case unset && len(args) > 0:
				return errors.New("--unset takes no deployment name: it clears your selection")
			case unset:
				return c.runUseUnset()
			case len(args) == 1:
				return c.runUse(args[0])
			case c.mayPrompt() && c.d.OutputTerminal != nil && c.d.OutputTerminal():
				return c.runUsePick()
			default:
				return c.runUseShow()
			}
		},
	}
	cmd.Flags().BoolVar(&unset, "unset", false, "Clear your selection")
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
	c.warnEnvOverridesPin(name)
	return r.Emit(useResult{Deployment: name, Status: "pinned"}, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "this project now uses %s, for you only (astro use --unset to clear)\n", name)
		return werr
	})
}

// warnEnvOverridesPin says so when ASTRO_DEPLOYMENT outranks the pin just
// written, since the pin then changes nothing until the variable is unset.
func (c *cli) warnEnvOverridesPin(pin string) {
	if env := os.Getenv(instances.EnvVar); env != "" && env != pin {
		fmt.Fprintf(c.d.Stderr, "note: %s=%s still takes precedence; unset it for %s to apply\n", instances.EnvVar, env, pin)
	}
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
		_, werr := fmt.Fprintln(w, "cleared your deployment selection for this project")
		return werr
	})
}

// runUsePick is bare `astro use` at a terminal: pick a link and pin it. The row
// this project resolves to now is highlighted, and a pin already written can be
// cleared from the same list, so the one question covers both directions.
func (c *cli) runUsePick() error {
	set, req, err := c.standing()
	if err != nil {
		return err
	}
	// Nothing to pick from: the report is the useful answer, since it says why
	// nothing resolves and lists the local Airflows running on this machine.
	if len(set.All()) == 0 {
		return c.runUseShow()
	}
	current, mark := "", ""
	if sel, serr := set.Select(instances.Request{Env: req.Env, Pin: req.Pin}); serr == nil {
		current, mark = sel.Instance.Name, currentMark(sel.From)
	}
	none := ""
	if req.Pin != "" {
		none = "clear your selection"
	}
	name, err := c.pickLink("Select the Deployment this project uses", none, current, mark, "name the Deployment to use: astro use NAME")
	if err != nil {
		return err
	}
	if name == "" {
		return c.runUseUnset()
	}
	return c.runUse(name)
}

// currentMark labels the picker's current row with what made it current, in
// the words the `astro deploy` prompt labels its highlight with, so one fact
// reads the same in both places. A row ASTRO_DEPLOYMENT chose is not one
// picking here will change, and one the manifest's default chose is not one
// the user ever selected, so both say where they came from. The user's own
// selection needs no label: the highlight already says it, and inside `astro
// use` there is nothing more to explain.
func currentMark(from instances.Layer) string {
	switch from {
	case instances.LayerEnv:
		return "← " + instances.EnvVar
	case instances.LayerDefault:
		return "← " + deploy.DefaultMarker
	case instances.LayerPin, instances.LayerFlag, instances.LayerURL:
	}
	return ""
}

// useListing is the bare `astro use` report off a terminal: the Deployments
// this project links, and which one its commands act on now. It is the
// picker's table without the question — the resolution rule behind the
// current row belongs in the help, not in every run.
type useListing struct {
	// Current is the Deployment commands act on now, "" when none resolves.
	Current string `json:"current,omitempty"`
	// From is what made Current current: "env" for ASTRO_DEPLOYMENT,
	// "selection" for `astro use`, "default" for the manifest's default link.
	From string `json:"from,omitempty"`
	// Reason explains an empty Current — several linked and no default, or a
	// selection naming a link that is gone — so the report is useful exactly
	// when resolution is stuck.
	Reason      string       `json:"reason,omitempty"`
	Deployments []useLinkRow `json:"deployments"`
}

// useLinkRow is one Deployment this project links.
type useLinkRow struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Where is the coordinate as the manifest writes it; URL is filled only
	// when the link already knows where it answers, so a consumer can tell a
	// resolved address from a coordinate that still needs a lookup.
	Where      string `json:"where,omitempty"`
	URL        string `json:"url,omitempty"`
	AuthMethod string `json:"auth_method,omitempty"`
	Current    bool   `json:"current"`
}

// layerFrom names a resolution layer for the report's "from" field: the
// layer's own name, except a selection's, which is "pin" — internal vocabulary
// this output does not use.
var layerFrom = map[instances.Layer]string{
	instances.LayerEnv:     string(instances.LayerEnv),
	instances.LayerPin:     "selection",
	instances.LayerDefault: string(instances.LayerDefault),
}

// noLinks is what the report prints for a project that links nothing, where an
// empty table would read as a bug.
const noLinks = "This project links no Deployments yet: link one with astro link add"

func (c *cli) runUseShow() error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	set, req, err := c.standing()
	if err != nil {
		return err
	}
	res := useListing{Deployments: []useLinkRow{}}
	var mark string
	sel, serr := set.Select(instances.Request{Env: req.Env, Pin: req.Pin})
	switch {
	case len(set.All()) == 0:
		res.Reason = noLinks
	case serr != nil:
		res.Reason = serr.Error()
	default:
		res.Current, res.From, mark = sel.Instance.Name, layerFrom[sel.From], currentMark(sel.From)
	}
	links := set.All()
	for i := range links {
		l := &links[i]
		res.Deployments = append(res.Deployments, useLinkRow{
			Name:       l.Name,
			Kind:       string(l.Kind),
			Where:      linkWhere(&l.Link),
			URL:        l.URL,
			AuthMethod: string(l.Link.Auth.Method),
			Current:    l.Name == res.Current,
		})
	}
	return r.Emit(res, func(w io.Writer) error { return renderUseListing(w, res, mark) })
}

// renderUseListing prints the links as the picker lists them, with a `*` on
// the current row in place of the picker's color — this output is often a pipe
// — and the same label when something other than your selection made it
// current.
func renderUseListing(w io.Writer, res useListing, mark string) error {
	if len(res.Deployments) == 0 {
		_, err := fmt.Fprintln(w, noLinks)
		return err
	}
	// The label column is blank on every row but one, so tabwriter's padding
	// leaves the rest ending in spaces; the table is built first and trimmed.
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "\tNAME\tKIND\tDEPLOYMENT, ENVIRONMENT OR URL\t")
	for _, row := range res.Deployments {
		marker, label := "", ""
		if row.Current {
			marker, label = "*", mark
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", marker, row.Name, row.Kind, dash(row.Where), label)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for line := range strings.Lines(table.String()) {
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " \n")); err != nil {
			return err
		}
	}
	if res.Reason != "" {
		_, err := fmt.Fprintf(w, "\nNo Deployment is current: %s\n", res.Reason)
		return err
	}
	return nil
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
