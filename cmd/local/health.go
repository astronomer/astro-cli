package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// newHealthCmd builds `health` over whichever Airflow the target names. It is
// a leaf rather than a family, so it registers the target's flags on itself.
func newHealthCmd(d Deps, t target) *cobra.Command {
	q := &query{cli: &cli{d: d}, t: t}
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Report an Airflow's version, import errors, DAG warnings, and run counts",
		Long: "Read the four things that answer \"is this Airflow in good shape\" on " + t.which() + ": what " +
			"version it runs, which DAG files failed to parse, what the scheduler is warning about, and how its " +
			"runs are distributed across states.\n\nEach part is read on its own and a part that fails is reported " +
			"as failed rather than ending the command, so an Airflow that serves three of the four still gives " +
			"you three. The command itself succeeds whenever it could produce a report; overall_status is the " +
			"verdict.\n\nFor the raw configuration this Airflow runs with, ask its /config endpoint: `" +
			rawAPIForm(t) + " /config`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runHealth(cmd.Context())
		},
	}
	attachTarget(q, cmd)
	return cmd
}

// rawAPIForm is the passthrough command on this surface: `astro local api` for
// the machine, and the spec-backed `astro api airflow` for a deployment. Help
// that sends a reader to the raw endpoint has to know which side it is on,
// and the two are spelled too differently to build from a prefix.
func rawAPIForm(t target) string {
	if _, machine := t.(machineTarget); machine {
		return "astro local api"
	}
	return "astro api airflow"
}

// versionRow is an Airflow's own account of itself, as the health report's
// version section carries it.
type versionRow struct {
	Version    string `json:"version,omitempty"`
	GitVersion string `json:"git_version,omitempty"`
	// Generation is the REST API generation serving this instance, "2" or "3".
	Generation string `json:"generation,omitempty"`
}

func newVersionRow(v airflowapi.VersionInfo) versionRow {
	return versionRow{
		Version:    v.Version,
		GitVersion: v.GitVersion,
		Generation: v.Generation.String(),
	}
}

// The verdicts a health report reaches, in the order severity runs.
const (
	healthUnhealthy = "unhealthy"
	healthWarning   = "warning"
	healthHealthy   = "healthy"
)

// healthReport is the composite `health` renders. Every section
// carries its own error, because the whole point is that one unserved endpoint
// does not cost you the other three.
type healthReport struct {
	Version      healthVersion      `json:"version"`
	ImportErrors healthImportErrors `json:"import_errors"`
	DAGWarnings  healthDAGWarnings  `json:"dag_warnings"`
	DAGStats     healthDAGStats     `json:"dag_stats"`
	// OverallStatus is healthy, warning, or unhealthy.
	OverallStatus string `json:"overall_status"`
	StatusReason  string `json:"status_reason"`
	// Unread names the sections that could not be read. It is the machine
	// readable half of StatusReason: a script filtering on overall_status has
	// to be able to see that the verdict rests on less than the whole report.
	Unread []string `json:"unread,omitempty"`
}

// healthVersion is the version section: what Airflow says about itself, plus
// why it could not be read.
type healthVersion struct {
	versionRow
	Error string `json:"error,omitempty"`
}

type healthImportErrors struct {
	Count  int              `json:"count"`
	Errors []importErrorRow `json:"errors,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type healthDAGWarnings struct {
	Count    int             `json:"count"`
	Warnings []dagWarningRow `json:"warnings,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type healthDAGStats struct {
	// Available is false when this Airflow does not serve run statistics,
	// which is a normal answer rather than a failure.
	Available bool         `json:"available"`
	Note      string       `json:"note,omitempty"`
	DAGs      []dagStatRow `json:"dags,omitempty"`
	// Runs is every run in DAGs, summed across states and DAGs. It is the
	// honest answer to "has anything run here", which len(DAGs) is not.
	Runs int `json:"runs"`
	// DAGsWithRuns counts the rows in DAGs whose own counts sum above zero.
	//
	// It exists because len(DAGs) means different things per generation, so
	// neither reading of it is reportable. Airflow 3 builds its response from
	// the rows of a DagRun query, so a DAG with no runs never appears. Airflow
	// 2 builds it from the REQUESTED ids, and pkg/airflowapi asks that
	// generation for every DAG on the instance (dags.go: DAGStats with no ids
	// lists them and joins the lot), so every DAG comes back whether it has run
	// or not. Both then zero-fill every DagRunState, so a row proves nothing by
	// existing.
	//
	// Counted here rather than in the renderer so the text and the json agree:
	// reporting len(DAGs) as "DAGs with runs" in one and shipping an array of
	// all-zero rows in the other described the same data two contradictory ways.
	DAGsWithRuns int    `json:"dags_with_runs"`
	Error        string `json:"error,omitempty"`
}

// importErrorRow is a DAG file the scheduler could not parse.
type importErrorRow struct {
	Filename   string `json:"filename"`
	StackTrace string `json:"stack_trace,omitempty"`
	Bundle     string `json:"bundle_name,omitempty"`
	Timestamp  string `json:"timestamp,omitempty"`
}

// dagWarningRow is something the scheduler noticed that did not stop a DAG
// from parsing.
type dagWarningRow struct {
	DAGID       string `json:"dag_id"`
	WarningType string `json:"warning_type,omitempty"`
	Message     string `json:"message,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

func (q *query) runHealth(ctx context.Context) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	// The four reads share nothing, so they go at once. Each carries a 30s
	// timeout, and in series one hung endpoint would hold the other three
	// behind it — the opposite of a report whose whole promise is that a
	// section which fails costs only itself. The client is safe to use
	// concurrently and collapses the generation probe across callers.
	var report healthReport
	reads := []func(){
		func() { report.Version = readHealthVersion(ctx, client) },
		func() { report.ImportErrors = readHealthImportErrors(ctx, client) },
		func() { report.DAGWarnings = readHealthDAGWarnings(ctx, client) },
		func() { report.DAGStats = readHealthDAGStats(ctx, client) },
	}
	var wg sync.WaitGroup
	wg.Add(len(reads))
	for _, read := range reads {
		go func() {
			defer wg.Done()
			read()
		}()
	}
	wg.Wait()

	report.OverallStatus, report.StatusReason = report.verdict()
	report.Unread = report.unread()
	if report.nothingWasReadable() {
		// No section answered, so there is nothing to render and nothing a
		// verdict could rest on. Fail, carrying up the reason underneath.
		return fmt.Errorf("%w\n%s", errHealthUnreadable, report.Version.Error)
	}
	return r.Emit(report, func(w io.Writer) error { return renderHealth(w, report) })
}

// sectionFailure is why a section could not be read, in the fewest words that
// stay true. A refusal is about the caller rather than the Airflow — some
// deployments grant no role the permission an endpoint needs — and a bare
// status sends the reader off to check their Airflow when the thing to change
// is the token. The section still counts as unread, because "not allowed to
// look" and "looked and it was clean" must not reach the same verdict.
func sectionFailure(err error) string {
	if errors.Is(err, airflowapi.ErrForbidden) {
		return "this token is not allowed to read it"
	}
	return err.Error()
}

func readHealthVersion(ctx context.Context, client *airflowapi.Client) healthVersion {
	info, err := client.Version(ctx)
	if err != nil {
		return healthVersion{Error: sectionFailure(err)}
	}
	return healthVersion{versionRow: newVersionRow(info)}
}

//nolint:dupl // see the note on readHealthDAGWarnings
func readHealthImportErrors(ctx context.Context, client *airflowapi.Client) healthImportErrors {
	list, err := client.ListImportErrors(ctx, airflowapi.ListOptions{})
	if err != nil {
		return healthImportErrors{Error: sectionFailure(err)}
	}
	rows := mapRows(list.ImportErrors, func(e airflowapi.ImportError) importErrorRow {
		return importErrorRow{
			Filename:   e.Filename,
			StackTrace: e.StackTrace,
			Bundle:     e.Bundle,
			Timestamp:  stamp(e.Timestamp),
		}
	})
	return healthImportErrors{Count: sectionCount(list.TotalEntries, len(rows)), Errors: rows}
}

// This and readHealthImportErrors are the same five moves over different types.
// They stay apart: the shapes they build are what the report promises its
// readers, and "errors" and "warnings" are different words to whoever reads it.
//
//nolint:dupl // see above
func readHealthDAGWarnings(ctx context.Context, client *airflowapi.Client) healthDAGWarnings {
	list, err := client.ListDAGWarnings(ctx, airflowapi.ListOptions{})
	if err != nil {
		return healthDAGWarnings{Error: sectionFailure(err)}
	}
	rows := mapRows(list.DAGWarnings, func(w airflowapi.DAGWarning) dagWarningRow {
		return dagWarningRow{
			DAGID:       w.DAGID,
			WarningType: w.WarningType,
			Message:     w.Message,
			Timestamp:   stamp(w.Timestamp),
		}
	})
	return healthDAGWarnings{Count: sectionCount(list.TotalEntries, len(rows)), Warnings: rows}
}

// sectionCount is how many a section found, which is what the instance says it
// has rather than how many fit on the page we asked for — 100 of 250 import
// errors is still 250 broken files. A server that omits the total falls back to
// what actually arrived, so the count is never softer than the evidence.
func sectionCount(total, fetched int) int {
	if total > fetched {
		return total
	}
	return fetched
}

func readHealthDAGStats(ctx context.Context, client *airflowapi.Client) healthDAGStats {
	stats, err := client.DAGStats(ctx, nil)
	if errors.Is(err, airflowapi.ErrNotServed) {
		// Not every Airflow has this endpoint, and the rest of the report is
		// worth having without it.
		return healthDAGStats{Note: notServedMessage("DAG run statistics")}
	}
	if err != nil {
		return healthDAGStats{Error: sectionFailure(err)}
	}
	section := healthDAGStats{Available: true, DAGs: dagStatRows(stats)}
	for _, row := range section.DAGs {
		runs := 0
		for _, n := range row.Stats {
			runs += n
		}
		section.Runs += runs
		if runs > 0 {
			section.DAGsWithRuns++
		}
	}
	return section
}

// verdict reduces the sections to one word and the sentence behind it: import
// errors outrank warnings, and anything else is healthy.
//
// A section that could not be read has no number to weigh, so it cannot make
// the verdict worse on its own evidence — but it must not let the verdict be
// "healthy" either. An unread section holds the answer back to at least
// `warning`: "I checked what I could and it was clean" and "I could not check"
// are different claims, and only the first one is good news.
func (r healthReport) verdict() (status, reason string) {
	switch {
	case r.ImportErrors.Count > 0:
		status = healthUnhealthy
		reason = fmt.Sprintf("%d DAG file(s) failed to parse", r.ImportErrors.Count)
	case r.DAGWarnings.Count > 0:
		status = healthWarning
		reason = fmt.Sprintf("%d DAG warning(s)", r.DAGWarnings.Count)
	default:
		status = healthHealthy
		reason = "no import errors or DAG warnings"
	}
	unread := r.unread()
	if len(unread) == 0 {
		return status, reason
	}
	if status == healthHealthy {
		// Nothing readable was wrong, but not everything was readable, so the
		// clean bill is withheld rather than issued on partial evidence.
		return healthWarning, fmt.Sprintf("not enough was read to judge (%s could not be read)",
			strings.Join(unread, " and "))
	}
	return status, reason + fmt.Sprintf(" (%s could not be read)", strings.Join(unread, " and "))
}

// errHealthUnreadable reports that every section failed. A report of nothing is
// not a report, so the command fails rather than printing four errors under a
// verdict it had no evidence for.
var errHealthUnreadable = errors.New("could not read anything from this Airflow: no version, import errors, DAG warnings, or run statistics")

// countedLine is one section's summary line: a label, its count, and the
// per-item lines under it.
type countedLine struct {
	label string
	count int
	// failure is why the section could not be read, empty when it could.
	failure string
	items   []string
}

// renderCounted writes one counted section, which import errors and DAG
// warnings render identically: the count, or why there isn't one, then a line
// per item.
func renderCounted(w io.Writer, section countedLine) error {
	if section.failure != "" {
		_, err := fmt.Fprintf(w, "%s: could not be read (%s)\n", section.label, section.failure)
		return err
	}
	if _, err := fmt.Fprintf(w, "%s: %d\n", section.label, section.count); err != nil {
		return err
	}
	for _, item := range section.items {
		if _, err := fmt.Fprintf(w, "  %s\n", item); err != nil {
			return err
		}
	}
	return nil
}

// unread names the sections that failed, so a reader knows the verdict rests
// on less than the whole report.
func (r healthReport) unread() []string {
	var names []string
	for _, section := range r.sections() {
		if section.err != "" {
			names = append(names, section.name)
		}
	}
	return names
}

// sections is what the report is made of, named for a reader. It is the one
// list both the verdict and the nothing-was-readable check count against, so
// adding a fifth section cannot leave either of them counting to four.
func (r healthReport) sections() []struct{ name, err string } {
	return []struct{ name, err string }{
		{"version", r.Version.Error},
		{"import errors", r.ImportErrors.Error},
		{"DAG warnings", r.DAGWarnings.Error},
		{"DAG statistics", r.DAGStats.Error},
	}
}

// nothingWasReadable reports that every section failed. A report of nothing is
// not a report.
func (r healthReport) nothingWasReadable() bool {
	return len(r.unread()) == len(r.sections())
}

func renderHealth(w io.Writer, report healthReport) error {
	if err := renderHealthVersion(w, report.Version); err != nil {
		return err
	}
	imports := report.ImportErrors
	if err := renderCounted(w, countedLine{
		label:   "import errors",
		count:   imports.Count,
		failure: imports.Error,
		items: mapRows(imports.Errors, func(e importErrorRow) string {
			return e.Filename + ": " + exceptionLine(e.StackTrace)
		}),
	}); err != nil {
		return err
	}
	warnings := report.DAGWarnings
	if err := renderCounted(w, countedLine{
		label:   "dag warnings",
		count:   warnings.Count,
		failure: warnings.Error,
		items: mapRows(warnings.Warnings, func(warning dagWarningRow) string {
			return warning.DAGID + ": " + firstLine(warning.Message)
		}),
	}); err != nil {
		return err
	}
	if err := renderHealthDAGStats(w, report.DAGStats); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nstatus: %s — %s\n", report.OverallStatus, report.StatusReason)
	return err
}

func renderHealthVersion(w io.Writer, section healthVersion) error {
	if section.Error != "" {
		_, err := fmt.Fprintf(w, "version: could not be read (%s)\n", section.Error)
		return err
	}
	_, err := fmt.Fprintf(w, "version: %s (api generation %s)\n", section.Version, section.Generation)
	return err
}

// renderHealthDAGStats writes the one line the report gives to run statistics.
//
// One write site and one prefix, so a branch cannot be added that forgets the
// "dag stats: " or emits a second line.
func renderHealthDAGStats(w io.Writer, section healthDAGStats) error {
	_, err := fmt.Fprintf(w, "dag stats: %s\n", dagStatsDetail(section))
	return err
}

func dagStatsDetail(section healthDAGStats) string {
	switch {
	case section.Error != "":
		return fmt.Sprintf("could not be read (%s)", section.Error)
	case !section.Available:
		if section.Note == "" {
			// Unreachable from readHealthDAGStats, which always pairs
			// Available: false with a Note or an Error — but a bare
			// "dag stats: " is a worse thing to print than a dull sentence.
			return "not available"
		}
		return section.Note
	// Runs, not "did any state key appear". Both Airflow generations zero-fill
	// every DagRunState on every row they return, so a state key proves only
	// that a row came back — which on Airflow 2 is true of every DAG on the
	// instance, run or not. Keyed off the count it means, a fresh project reads
	// "no runs" on both generations instead of claiming one DAG has runs and
	// then printing four zeros.
	case section.Runs == 0:
		return "no runs"
	}
	// One line per state across every DAG: the health question is "how are runs
	// going here", which is a total, not a per-DAG table. Zero states stay in —
	// failed=0 is an answer, and cmd/local/query.go makes the same call.
	totals := map[string]int{}
	for _, row := range section.DAGs {
		for state, n := range row.Stats {
			totals[state] += n
		}
	}
	parts := make([]string, 0, len(totals))
	for _, state := range slices.Sorted(maps.Keys(totals)) {
		parts = append(parts, fmt.Sprintf("%s=%d", state, totals[state]))
	}
	// DAGsWithRuns rather than len(section.DAGs): see the field's comment for
	// why the length is not reportable as either reading.
	return fmt.Sprintf("%d DAG(s) with runs, %s", section.DAGsWithRuns, strings.Join(parts, " "))
}
