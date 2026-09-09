package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	projectpkg "github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// ExitError carries a process exit code up to main, which is the only place
// that exits. The command has already rendered everything the user needs, so
// main propagates the code without printing again (the cmd/otto.go pattern).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func newCheckCmd(c *cli) *cobra.Command {
	var strict bool
	var targets []string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate this project's DAGs without starting Airflow",
		Long: "Parse the project's DAGs in its own environment and report import errors, duplicate DAG ids, and slow parses. Runs offline; starts no Airflow.\n\n" +
			"With --target, check the project against the Airflow a managed platform actually runs, before you upload. mwaa and composer map the manifest's Airflow pin to the closest version that platform offers, build a scratch venv with that Airflow plus the project's dependencies, and parse the DAGs inside it; mwaa also resolves the dependencies against MWAA's published constraints file (this step needs the network and is skipped, not failed, offline). astro is the default check under a name, so --target astro is an alias for a plain check. The flag repeats and takes a comma list: --target mwaa --target composer or --target mwaa,composer.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(targets) == 0 {
				return c.runCheck(cmd.Context(), strict)
			}
			return c.runTargetCheck(cmd.Context(), targets, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as failures")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "Check against a platform's Airflow before upload: astro (default check), mwaa, or composer. Repeatable, and takes a comma list.")
	return cmd
}

func (c *cli) runCheck(ctx context.Context, strict bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	project, err := c.projectPath()
	if err != nil {
		return err
	}

	res, err := checks.Run(ctx, checks.Options{ProjectPath: project, Strict: strict}, c.d.Checks)
	if err != nil {
		if errors.Is(err, checks.ErrEnvNotReady) {
			if rerr := renderEnvNotReady(r, err); rerr != nil {
				return rerr
			}
			return &ExitError{Code: checks.ExitEnvNotReady}
		}
		return err
	}

	if err := renderCheck(r, res, strict); err != nil {
		return err
	}
	if code := res.ExitCode(strict); code != checks.ExitOK {
		return &ExitError{Code: code}
	}
	return nil
}

// runTargetCheck runs one or more target-aware pre-flight checks and renders a
// report per target. It picks a single exit code: the worst across the targets
// (operational error over findings over clean), so CI fails on any of them.
func (c *cli) runTargetCheck(ctx context.Context, targets []string, strict bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	for _, t := range targets {
		if !checks.KnownTarget(t) {
			return fmt.Errorf("unknown target %q (supported: astro, mwaa, composer)", t)
		}
	}
	project, err := c.projectPath()
	if err != nil {
		return err
	}
	m, err := manifest.Load(filepath.Join(project, projectpkg.Marker))
	if err != nil {
		return err
	}

	reports := make([]checks.TargetReport, 0, len(targets))
	worst := checks.ExitOK
	for _, t := range dedupeTargets(targets) {
		rep := c.checkTarget(ctx, t, project, m, r, strict)
		reports = append(reports, rep)
		if code := rep.ExitCode(strict); code > worst {
			worst = code
		}
	}

	if err := renderTargetChecks(r, reports, strict); err != nil {
		return err
	}
	if worst != checks.ExitOK {
		return &ExitError{Code: worst}
	}
	return nil
}

// checkTarget runs one target. astro reuses today's project-venv check, wrapped
// as a target report; mwaa and composer run the scratch-venv pre-flight. The
// progress notes stream in text mode and are dropped in json mode, where the
// report object carries everything.
func (c *cli) checkTarget(ctx context.Context, target, project string, m *manifest.Manifest, r Renderer, strict bool) checks.TargetReport {
	progress := c.progressFn(r, target)
	if target == checks.TargetAstro {
		return c.checkAstroTarget(ctx, project, m, strict)
	}

	prov, err := c.provisioner(ctx)
	if err != nil {
		return checks.TargetReport{Target: target, OpError: err.Error()}
	}
	return checks.Preflight(ctx, target, checks.PreflightInput{
		ProjectPath: project,
		DagsDir:     checks.DefaultDagsDir(project),
		Pin:         m.Astro.AirflowVersion,
		Deps:        m.Project.Dependencies,
	}, prov, c.d.CheckVenv, strict, progress)
}

// checkAstroTarget runs the plain project-venv check and shapes it as a target
// report, so --target astro reads uniformly beside the platform targets. Its
// verdict, findings, and env-not-ready handling are today's check exactly.
func (c *cli) checkAstroTarget(ctx context.Context, project string, m *manifest.Manifest, strict bool) checks.TargetReport {
	rep := checks.TargetReport{
		Target:         checks.TargetAstro,
		AirflowChecked: m.Astro.AirflowVersion,
		Notes:          []string{"astro runs your project's own Airflow; this is the default `astro local check`"},
	}
	res, err := checks.Run(ctx, checks.Options{ProjectPath: project, Strict: strict}, c.d.Checks)
	if err != nil {
		rep.OpError = err.Error()
		return rep
	}
	rep.Findings = res.Findings
	if rep.Findings == nil {
		rep.Findings = []checks.Finding{}
	}
	rep.DagCount = res.DagCount
	rep.Errors = res.Errors
	rep.Warnings = res.Warnings
	return rep
}

// provisioner builds the scratch-venv provisioner, honoring a test-injected
// factory and falling back to the uv-backed production one.
func (c *cli) provisioner(ctx context.Context) (checks.Provisioner, error) {
	if c.d.Provisioner != nil {
		return c.d.Provisioner(ctx)
	}
	return newUVProvisioner(ctx)
}

// progressFn streams a target's progress notes in text mode and drops them in
// json mode, where the final report object is the whole output.
func (c *cli) progressFn(r Renderer, target string) func(string) {
	if r.Format == FormatJSON {
		return func(string) {}
	}
	return func(note string) {
		// Progress is best-effort; a broken pipe surfaces on the final write.
		fmt.Fprintf(r.Out, "[%s] %s\n", target, note)
	}
}

// dedupeTargets keeps the first occurrence of each target, so --target
// mwaa,mwaa checks it once.
func dedupeTargets(targets []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// checkSummary closes a check run: the counts, and the verdict under the
// strictness in force. In --output json it is the final NDJSON line.
type checkSummary struct {
	Event    string `json:"event"`
	Dags     int    `json:"dags"`
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
	Strict   bool   `json:"strict"`
	Passed   bool   `json:"passed"`
}

// renderCheck writes findings then a summary. In json mode each finding is one
// NDJSON line and the summary is the last; in text mode findings form a table
// and the summary is one sentence. Both render the same data.
func renderCheck(r Renderer, res checks.Result, strict bool) error {
	summary := checkSummary{
		Event:    "summary",
		Dags:     res.DagCount,
		Errors:   res.Errors,
		Warnings: res.Warnings,
		Strict:   strict,
		Passed:   res.Passed(strict),
	}
	if r.Format == FormatJSON {
		enc := json.NewEncoder(r.Out)
		for _, f := range res.Findings {
			if err := enc.Encode(f); err != nil {
				return err
			}
		}
		return enc.Encode(summary)
	}
	if err := renderFindingsTable(r.Out, res.Findings); err != nil {
		return err
	}
	_, err := fmt.Fprintln(r.Out, checkSummaryLine(summary))
	return err
}

func renderFindingsTable(w io.Writer, findings []checks.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEVERITY\tCHECK\tLOCATION\tDETAIL")
	for _, f := range findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Severity, f.Kind, findingLocation(f), findingDetail(f))
	}
	return tw.Flush()
}

func findingLocation(f checks.Finding) string {
	if f.DagID != "" {
		return f.DagID
	}
	return f.File
}

func findingDetail(f checks.Finding) string {
	switch f.Kind {
	case checks.KindImportError:
		return firstLine(f.Message)
	case checks.KindDuplicateDagID:
		return "defined in " + strings.Join(f.Files, ", ")
	case checks.KindSlowParse:
		return fmt.Sprintf("parsed in %.1fs (threshold %.0fs)", f.ParseSeconds, f.ThresholdSeconds)
	case checks.KindChecksIncomplete:
		// The whole finding is its message: it names checks that did not run,
		// so there is no file or dag_id to point at and the LOCATION column
		// stays empty.
		return f.Message
	default:
		return f.Message
	}
}

// firstLine keeps a table row to one line; an import error's full traceback
// stays available in --output json.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func checkSummaryLine(s checkSummary) string {
	verdict := "passed"
	if !s.Passed {
		verdict = "failed"
	}
	tail := ""
	if s.Strict {
		tail = " (strict)"
	}
	return fmt.Sprintf("checks %s%s: %d DAGs, %d errors, %d warnings", verdict, tail, s.Dags, s.Errors, s.Warnings)
}

// renderEnvNotReady reports that the environment could not be inspected. In
// json mode it is a single structured line; in text mode, the guidance.
func renderEnvNotReady(r Renderer, err error) error {
	msg := err.Error()
	if r.Format == FormatJSON {
		return json.NewEncoder(r.Out).Encode(struct {
			Event   string `json:"event"`
			Message string `json:"message"`
		}{Event: "error", Message: msg})
	}
	_, werr := fmt.Fprintln(r.Out, msg)
	return werr
}

// renderTargetChecks writes one block per target. In json mode each target is
// one NDJSON line (the TargetReport, findings array and all); in text mode each
// is a headed block with the version mapping, a findings table, any constraints
// result, and a one-line verdict.
func renderTargetChecks(r Renderer, reports []checks.TargetReport, strict bool) error {
	if r.Format == FormatJSON {
		enc := json.NewEncoder(r.Out)
		for i := range reports {
			if err := enc.Encode(reports[i]); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range reports {
		if i > 0 {
			if _, err := fmt.Fprintln(r.Out); err != nil {
				return err
			}
		}
		if err := renderTargetReport(r.Out, &reports[i], strict); err != nil {
			return err
		}
	}
	return nil
}

func renderTargetReport(w io.Writer, rep *checks.TargetReport, strict bool) error {
	if _, err := fmt.Fprintf(w, "== %s ==\n", rep.Target); err != nil {
		return err
	}
	if rep.OpError != "" {
		_, err := fmt.Fprintf(w, "could not check: %s\n", rep.OpError)
		return err
	}
	if rep.MappedFrom != "" {
		if _, err := fmt.Fprintf(w, "checking against Airflow %s (mapped down from the manifest pin %s)\n", rep.AirflowChecked, rep.MappedFrom); err != nil {
			return err
		}
	} else if rep.AirflowChecked != "" {
		if _, err := fmt.Fprintf(w, "checking against Airflow %s\n", rep.AirflowChecked); err != nil {
			return err
		}
	}
	for _, note := range rep.Notes {
		if _, err := fmt.Fprintf(w, "note: %s\n", note); err != nil {
			return err
		}
	}
	if len(rep.Findings) == 0 {
		if _, err := fmt.Fprintln(w, "no DAG findings"); err != nil {
			return err
		}
	} else if err := renderFindingsTable(w, rep.Findings); err != nil {
		return err
	}
	if err := renderConstraintOutcome(w, rep.Constraints); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, targetVerdictLine(rep, strict))
	return err
}

// renderConstraintOutcome prints the MWAA constraints result: a clean resolve, a
// conflict with the resolver's message, or the skip note when the file could
// not be fetched.
func renderConstraintOutcome(w io.Writer, c *checks.ConstraintOutcome) error {
	if c == nil {
		return nil
	}
	switch {
	case c.OK:
		_, err := fmt.Fprintln(w, "constraints: dependencies resolve against MWAA's constraints file")
		return err
	case c.Conflict != "":
		_, err := fmt.Fprintf(w, "constraints: conflict — %s\n", c.Conflict)
		return err
	default:
		_, err := fmt.Fprintf(w, "constraints: %s\n", c.Skipped)
		return err
	}
}

func targetVerdictLine(rep *checks.TargetReport, strict bool) string {
	verdict := "passed"
	if rep.ExitCode(strict) != checks.ExitOK {
		verdict = "failed"
	}
	tail := ""
	if strict {
		tail = " (strict)"
	}
	return fmt.Sprintf("check %s%s for %s: %d DAGs, %d errors, %d warnings", verdict, tail, rep.Target, rep.DagCount, rep.Errors, rep.Warnings)
}
