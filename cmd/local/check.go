package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/checks"
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
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate this project's DAGs without starting Airflow",
		Long:  "Parse the project's DAGs in its own environment and report import errors, duplicate DAG ids, and slow parses. Runs offline; starts no Airflow.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runCheck(cmd.Context(), strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as failures")
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
