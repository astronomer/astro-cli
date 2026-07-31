package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// query is one Airflow-facing command family: the cli it renders through, and
// the instance flags its leaves share.
type query struct {
	*cli
	f instanceFlags
}

// instanceEnvSentence closes every query family's help with the same account of
// what picks the Airflow, so one rule is described in one place.
const instanceEnvSentence = "then " + instances.EnvVar + ", then `astro use`, then the local Airflow running for " +
	"this project, then the manifest's default link. --url reaches an Airflow no project declares."

// newQueryCmd finishes a top-level family that acts on an instance: -i/--url
// and --output on the parent, the bare-family-prints-help posture, the
// skip-pre-run annotation over the whole subtree, and the leaves each builder
// returns. Every query family is one call to this, so the flags and the help
// posture cannot drift apart between them.
func newQueryCmd(d Deps, cmd *cobra.Command, builders ...func(*query) *cobra.Command) *cobra.Command {
	q := &query{cli: &cli{d: d}}
	cmd.Args = cobra.ArbitraryArgs
	// A bare family prints help and succeeds; a typo fails. Cobra returns help
	// before it validates args on a non-runnable parent, so both cases are
	// driven here — the same shape `astro local` uses.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	addInstanceFlags(cmd, &q.f)
	addOutputFlag(cmd, &q.cli.output)
	// Every leaf is built over the one query, so they share the family's flags.
	for _, build := range builders {
		cmd.AddCommand(build(q))
	}
	markSkipPreRun(cmd)
	return cmd
}

// open is the first line of every query leaf: validate the output format,
// resolve which Airflow this command acts on, and open a client on it. The
// format is checked first so a misspelled --output fails before anything
// reaches the network.
func (q *query) open(ctx context.Context) (Renderer, *airflowapi.Client, error) {
	r, err := q.renderer()
	if err != nil {
		return Renderer{}, nil, err
	}
	_, client, err := q.instanceClient(ctx, q.f)
	if err != nil {
		return Renderer{}, nil, err
	}
	return r, client, nil
}

// listFlags is the pagination and ordering every Airflow list endpoint takes.
// --offset carries no shorthand because -o is --output across the whole v2
// tree.
type listFlags struct {
	limit   int
	offset  int
	orderBy string
}

// addListFlags registers pagination on a list command. defaultOrder is the
// sort a reader wants without asking — most recent first, where the endpoint
// has a time to sort on — and "" leaves the order to Airflow.
func addListFlags(cmd *cobra.Command, f *listFlags, defaultOrder string) {
	cmd.Flags().IntVarP(&f.limit, "limit", "l", airflowapi.DefaultLimit, "Maximum rows to return")
	cmd.Flags().IntVar(&f.offset, "offset", 0, "Row to start the page at")
	cmd.Flags().StringVar(&f.orderBy, "order-by", defaultOrder, "Sort field; prefix with - for descending")
}

func (f listFlags) options() airflowapi.ListOptions {
	return airflowapi.ListOptions{Limit: f.limit, Offset: f.offset, OrderBy: f.orderBy}
}

// notServedMessage is what an Airflow without an endpoint is reported as. What
// an instance serves is discovered by asking, so this is a normal answer on an
// older or trimmed-down Airflow rather than a fault to dump a status for.
func notServedMessage(what string) string {
	return "this Airflow does not serve " + what
}

// notServed turns "this Airflow does not have that endpoint" into a sentence
// naming what is missing, and passes every other failure through untouched.
func notServed(what string, err error) error {
	if errors.Is(err, airflowapi.ErrNotServed) {
		return errors.New(notServedMessage(what))
	}
	return err
}

// mapRows turns a page of wire values into the rows this surface renders.
// Every list command needs exactly this, and only the row differs.
func mapRows[Wire, Row any](wire []Wire, row func(Wire) Row) []Row {
	rows := make([]Row, 0, len(wire))
	for i := range wire {
		rows = append(rows, row(wire[i]))
	}
	return rows
}

// Each family declares its own row type rather than emitting the client's,
// even where the two carry the same fields. pkg/airflowapi is a separately
// versioned sub-module, and what `--output json` prints is a contract with
// whoever is parsing it; a row type is the seam that keeps a retag over there
// from moving a key over here. Several rows earn it twice over — folding the
// two API generations' spellings into one field, rendering times and durations
// once so text and json agree, and leaving a connection's password with no
// field to land in. The one exception is `astro instance config`, whose shape
// is sections of free-form key/value pairs that a twin could only restate.

// field is one line of a detail rendering: a label and its value. Labels are
// prose rather than wire keys, matching `astro local status`; the wire keys are
// what --output json is for.
type field struct {
	name  string
	value string
}

// renderFields writes a detail view as aligned label/value lines. An empty
// value is dropped rather than shown as a dash: the two API generations send
// different fields, and a column of blanks for what this Airflow never had is
// noise a reader has to learn to ignore.
func renderFields(w io.Writer, fields []field) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		fmt.Fprintf(tw, "%s:\t%s\n", f.name, f.value)
	}
	return tw.Flush()
}

// emitDetail renders one object: the value itself in json mode, its fields as
// label/value lines in text.
func emitDetail[T any](r Renderer, v T, fields func(T) []field) error {
	return r.Emit(v, func(w io.Writer) error { return renderFields(w, fields(v)) })
}

// renderTable writes rows through a tabwriter under headers, with empty
// message when there are none. cells returns one row's columns, already
// rendered; a cell that is empty shows as a dash so a short row still lines up
// with the header above it.
func renderTable[T any](w io.Writer, rows []T, empty string, headers []string, cells func(T) []string) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, empty)
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		// Copy before filling blanks: cells may hand back a slice the row
		// itself owns, and dashing it in place would rewrite the data.
		columns := append([]string(nil), cells(row)...)
		for i, cell := range columns {
			columns[i] = dash(cell)
		}
		fmt.Fprintln(tw, strings.Join(columns, "\t"))
	}
	return tw.Flush()
}

// writeText writes a command's output verbatim, ending it with a newline it
// may not carry. It backs the two commands whose text mode is the thing itself
// rather than a rendering of it — a DAG's source and a task's log — because
// both get piped into a file or a pager.
func writeText(w io.Writer, text string) error {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_, err := io.WriteString(w, text)
	return err
}

// stamp renders a time for display. Absent dates arrive from Airflow as null
// and reach here as the zero value, which renders as nothing rather than as
// year one.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// span is how long something took, in seconds, from its two ends. It is zero
// until both are known, so a running task reports no duration rather than a
// wrong one, and zero for a negative span: an end before its start is clock
// skew between Airflow's workers, not a run that took negative time.
func span(start, end time.Time) float64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	elapsed := end.Sub(start).Seconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// formatDuration renders a duration for a person. It is the text half of a
// field whose json half is the number itself, so a consumer can do arithmetic
// on what Airflow measured while a reader gets something scannable.
//
// Sub-second durations get their own unit: plenty of tasks finish inside a
// second, and rounding all of them to "0s" throws away the only figure that
// distinguishes them.
func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Second {
		return fmt.Sprintf("%dms", max(d.Milliseconds(), 1))
	}
	return formatUptime(d)
}

// yesNo renders a boolean for a table column, where "false" and an empty cell
// have to be told apart at a glance.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// count renders a number for a detail line or a table cell.
func count(n int) string {
	return strconv.Itoa(n)
}

// omitZero renders a number, leaving zero blank so the detail line is dropped.
// It is for the counts where zero is not a real answer — a limit of zero, a
// port of zero — never for one where it is, such as a retry count.
func omitZero(n int) string {
	if n == 0 {
		return ""
	}
	return count(n)
}

// onlyIf renders value when the condition holds, so a detail line appears only
// when it has something to report.
func onlyIf(cond bool, value string) string {
	if cond {
		return value
	}
	return ""
}

// firstNonEmpty is the first value that was sent. The two API generations
// spell several fields differently and send only their own, so this is how a
// row folds both spellings into one.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
