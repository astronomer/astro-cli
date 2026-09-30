package local

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// loadCheckedManifest loads the project's manifest the way every run path
// does: validated, and with a declared Dockerfile's FROM held to the Airflow
// requirement (scaffold.CheckDockerfileAirflow). A check validates the project
// it would run, so a project that `astro local start` refuses is not one this
// passes.
func loadCheckedManifest(project string) (*manifest.Manifest, error) {
	m, err := manifest.Load(filepath.Join(project, manifest.Marker))
	if err != nil {
		return nil, err
	}
	if err := scaffold.CheckDockerfileAirflow(project, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ExitError carries a process exit code up to main, which is the only place
// that exits. The command has already rendered everything the user needs, so
// main propagates the code without printing again (the cmd/otto.go pattern).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// nameCheck is the command's name, and the label its progress notes carry.
const nameCheck = "check"

func newCheckCmd(c *cli) *cobra.Command {
	var strict bool
	var targets []string
	cmd := &cobra.Command{
		Use:   nameCheck,
		Short: "Validate this project's DAGs without starting Airflow",
		Long: "Parse the project's DAGs in its own environment and report import errors, duplicate DAG ids, and slow parses. Runs offline; starts no Airflow.\n\n" +
			"It also checks [tool.astro.env] the way astro local start does: a required value with no source on this machine is an error, and a value that is not what its declaration says is a warning. A source = 'workspace' value nothing local holds is not checked, since the Environment Manager is not asked.\n\n" +
			"With --target, check the project against the Airflow a managed platform actually runs, before you upload. mwaa and composer map the manifest's Airflow pin to the closest version that platform offers, build a scratch venv with that Airflow plus the project's dependencies, and parse the DAGs inside it; mwaa also resolves the dependencies against MWAA's published constraints file (a conflict fails the check; the step needs the network and is skipped, not failed, offline). astro is the default check under a name, so --target astro is an alias for a plain check. The flag repeats and takes a comma list: --target mwaa --target composer or --target mwaa,composer.",
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
		// Not in a project directory. No verdict was reached, so it takes the
		// same door as every other outcome of that kind rather than falling
		// through to cobra and exiting 1.
		return blocked(r, err)
	}

	// The manifest is validated before the environment is touched: a manifest
	// problem is certain, cheap and offline, while a missing venv is expensive
	// to fix and may not be the real problem. It also makes `--target astro` a
	// true alias for a plain check, since that path loads the manifest too.
	m, err := loadCheckedManifest(project)
	if err != nil {
		return blocked(r, err)
	}
	env, envRep, err := plan.EnvironReport(project, m)
	if err != nil {
		return blocked(r, err)
	}

	// Collect the cached check environments once this run is finished with
	// them — see sweepCheckVenvs for why not from inside EnsureVenv. Deferred,
	// so it also covers the paths that give up partway: a run that failed
	// still used whatever it resolved, and the stamp on that one is what
	// protects it.
	defer sweepCheckVenvs(c.sweepProgressFn(r))

	res, provisioned, err := c.check(ctx, r, checks.Options{ProjectPath: project, Strict: strict, Env: env}, m)
	if err != nil {
		if errors.Is(err, checks.ErrEnvNotReady) {
			return blocked(r, err)
		}
		return err
	}
	res = withEnvFindings(res, envFindings(envRep))

	// Best effort: the check is about the DAGs, and a listing that cannot be
	// read only costs the note.
	undeclared, _ := plan.UndeclaredLocal(project, m) //nolint:errcheck // informational, see above
	if err := renderCheck(r, res, strict, provisioned, undeclared); err != nil {
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
			return blocked(r, fmt.Errorf("unknown target %q (supported: astro, mwaa, composer)", t))
		}
	}
	project, err := c.projectPath()
	if err != nil {
		return blocked(r, err)
	}
	m, err := loadCheckedManifest(project)
	if err != nil {
		return blocked(r, err)
	}
	env, envRep, err := plan.EnvironReport(project, m)
	if err != nil {
		return blocked(r, err)
	}

	// Once, after every target — not per target. Each resolves its own
	// environment, and a sweep between two of them deletes what the next is
	// about to use. See sweepCheckVenvs.
	defer sweepCheckVenvs(c.sweepProgressFn(r))

	reports := make([]checks.TargetReport, 0, len(targets))
	worst := checks.ExitOK
	for _, t := range dedupeTargets(targets) {
		rep := c.checkTarget(ctx, t, project, env, m, r, strict)
		// The declared environment is this machine's, which only the astro
		// target runs under; mwaa and composer set theirs on the platform,
		// from the ENV_SETUP.md checklist `astro package` writes.
		if t == checks.TargetAstro && rep.OpError == "" {
			res := withEnvFindings(checks.Result{Findings: rep.Findings, DagCount: rep.DagCount, Errors: rep.Errors, Warnings: rep.Warnings}, envFindings(envRep))
			rep.Findings, rep.Errors, rep.Warnings = res.Findings, res.Errors, res.Warnings
		}
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
func (c *cli) checkTarget(ctx context.Context, target, project string, env []string, m *manifest.Manifest, r Renderer, strict bool) checks.TargetReport {
	progress := c.progressFn(r, target)
	if target == checks.TargetAstro {
		return c.checkAstroTarget(ctx, project, env, m, r, strict)
	}

	prov, err := c.provisioner(ctx)
	if err != nil {
		return checks.TargetReport{Target: target, OpError: err.Error()}
	}
	// Composer installs the requirements `astro package composer` writes, which
	// keep the bare names, since gcloud takes no direct references.
	deps := m.Requirements()
	if target == checks.TargetComposer {
		deps = m.Project.Dependencies
	}
	return checks.Preflight(ctx, target, checks.PreflightInput{
		ProjectPath: project,
		DagsDir:     checks.DefaultDagsDir(project),
		Pin:         m.Airflow().Pin,
		Deps:        deps,
		// Without the pins to Astronomer's build of Airflow: a platform runs
		// Apache's own, at its own version.
		Constraints: manifest.WithoutAstroPins(m.UV.ConstraintDependencies),
		Env:         env,
	}, prov, c.d.CheckVenv, strict, progress)
}

// checkAstroTarget runs the plain project-venv check and shapes it as a target
// report, so --target astro reads uniformly beside the platform targets. Its
// verdict, findings, and env-not-ready handling are today's check exactly.
func (c *cli) checkAstroTarget(ctx context.Context, project string, env []string, m *manifest.Manifest, r Renderer, strict bool) checks.TargetReport {
	rep := checks.TargetReport{
		Target:         checks.TargetAstro,
		AirflowChecked: m.Airflow().Pin,
		Notes:          []string{"astro runs your project's own Airflow; this is the default `astro local check`"},
	}
	res, provisioned, err := c.check(ctx, r, checks.Options{ProjectPath: project, Strict: strict, Env: env}, m)
	if err != nil {
		rep.OpError = err.Error()
		return rep
	}
	if provisioned {
		rep.Notes = append(rep.Notes, "checked in an environment built from the manifest; this project has none of its own")
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

// check runs the project's DAG checks, building an interpreter first when the
// project has none of its own.
//
// One function because `--target astro` is documented as an alias for the
// plain check and has to stay one: routing only the plain path through the
// fallback would have a docker-mode project pass here and report "environment
// not ready" there, for the same project in the same state.
func (c *cli) check(ctx context.Context, r Renderer, opts checks.Options, m *manifest.Manifest) (res checks.Result, provisioned bool, err error) {
	res, err = checks.Run(ctx, opts, c.d.Checks)
	// Only "there is no interpreter". ErrEnvNotReady also covers one whose
	// Airflow will not import, and building a fresh environment for that would
	// check the project against dependencies it does not have installed — a
	// green check for a project whose own environment is broken.
	if errors.Is(err, checks.ErrNoInterpreter) {
		res, err = c.checkWithBuiltEnv(ctx, r, opts, m)
		return res, true, err
	}
	return res, false, err
}

// checkWithBuiltEnv runs the check against an interpreter built for it.
//
// A provisioning failure is reported as ErrEnvNotReady joined with what went
// wrong, and joined with the reason there was no interpreter to begin with:
// "no uv on this machine" alone leaves out that a start would also have fixed
// this, which for a standalone project is the shorter road.
func (c *cli) checkWithBuiltEnv(ctx context.Context, r Renderer, opts checks.Options, m *manifest.Manifest) (checks.Result, error) {
	prov, err := c.provisioner(ctx)
	if err != nil {
		return checks.Result{}, errors.Join(noInterpreter(opts.ProjectPath), err)
	}
	res, err := checks.RunProvisioned(ctx, opts, checks.ProvisionInput{
		ProjectPath: opts.ProjectPath,
		DagsDir:     checks.DefaultDagsDir(opts.ProjectPath),
		Pin:         m.Airflow().Pin,
		Deps:        m.Requirements(),
		// The venv is built outside the project, where uv cannot read the
		// manifest, so a stated requires-python is passed through. Without one
		// the check takes the interpreter a start would, rather than whatever
		// uv finds newest.
		RequiresPython: cmp.Or(m.Project.RequiresPython,
			airflowrt.PythonFallback(m.Project.RequiresPython, m.Airflow().Pin)),
		Constraints: m.UV.ConstraintDependencies,
		FindLinks:   m.UV.IndexPages(),
	}, prov, c.d.CheckVenv, c.progressFn(r, nameCheck))
	if err != nil && !errors.Is(err, checks.ErrEnvNotReady) {
		// An operational failure of the parse itself is not an environment
		// problem, and relabelling it as one would give the same crash a
		// different exit code here than it gets against a project's own venv.
		return checks.Result{}, err
	}
	if err != nil {
		return checks.Result{}, errors.Join(noInterpreter(opts.ProjectPath), err)
	}
	return res, nil
}

// noInterpreter is the reason the fallback ran, kept so a failure to build one
// still says what was missing and what else would have supplied it.
func noInterpreter(projectPath string) error {
	return fmt.Errorf("%w: no Python at %s, and building one did not work",
		checks.ErrNoInterpreter, checks.VenvInterpreter(filepath.Join(projectPath, ".venv")))
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

// sweepProgressFn is where the cache sweep's notes go, and it differs from a
// target's progress in one way that matters: these report a deletion, so
// dropping them in json mode would have the command remove hundreds of
// megabytes and record it nowhere. They go to stderr instead, which leaves the
// report object on stdout parseable.
func (c *cli) sweepProgressFn(r Renderer) func(string) {
	if r.Format == FormatJSON {
		return func(note string) {
			fmt.Fprintf(c.d.Stderr, "[%s] %s\n", nameCheck, note)
		}
	}
	return c.progressFn(r, nameCheck)
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
	// Provisioned reports that the parse ran in an environment this command
	// built, rather than the project's own.
	//
	// A json consumer cannot otherwise tell the two apart, and they are not
	// equivalent: a built environment is resolved from the manifest, so it can
	// disagree with what the project actually runs — most obviously for a
	// docker project, whose image is the real environment. Omitted when false
	// so the ordinary payload is unchanged.
	Provisioned bool `json:"provisioned,omitempty"`
	// UndeclaredEnv is the env-var names the project gets from this machine
	// without declaring them (plan.UndeclaredLocal). Informational: they work
	// here and will not follow the project to a Deployment or a teammate, so
	// they never fail the check, --strict or not. Names only.
	UndeclaredEnv []string `json:"undeclared_env,omitempty"`
}

// renderCheck writes findings then a summary. In json mode each finding is one
// NDJSON line and the summary is the last; in text mode findings form a table
// and the summary is one sentence. Both render the same data.
func renderCheck(r Renderer, res checks.Result, strict, provisioned bool, undeclared []string) error {
	summary := checkSummary{
		Event:         "summary",
		Provisioned:   provisioned,
		UndeclaredEnv: undeclared,
		Dags:          res.DagCount,
		Errors:        res.Errors,
		Warnings:      res.Warnings,
		Strict:        strict,
		Passed:        res.Passed(strict),
	}
	if r.Format == FormatJSON {
		for i := range res.Findings {
			if err := r.Emit(res.Findings[i], nil); err != nil {
				return err
			}
		}
		return r.Emit(summary, nil)
	}
	budget := maxTracebacks
	if err := renderFindingsTable(r.Out, res.Findings, &budget); err != nil {
		return err
	}
	// The blank line before the verdict is the caller's, not the findings
	// block's. Emitting it from whichever part happened to print last made the
	// spacing depend on the KIND of finding — an import error with frames got a
	// gap, a duplicate dag id butted straight up against the verdict.
	if len(res.Findings) > 0 {
		if _, err := fmt.Fprintln(r.Out); err != nil {
			return err
		}
	}
	if note := plan.UndeclaredNote(undeclared); note != "" {
		if _, err := fmt.Fprintf(r.Out, "info: %s\n\n", note); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(r.Out, checkSummaryLine(summary))
	return err
}

func renderFindingsTable(w io.Writer, findings []checks.Finding, budget *int) error {
	if len(findings) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEVERITY\tCHECK\tLOCATION\tDETAIL")
	for i := range findings {
		f := &findings[i]
		// Both text columns are sanitized, not just DETAIL: a dag_id comes from
		// somebody's Python and a filename may legally contain a tab, and either
		// one opens a phantom column that shifts every row below it.
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Severity, f.Kind, cell(findingLocation(*f)), cell(findingDetail(*f)))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return renderTracebacks(w, findings, budget)
}

// cellReplacer collapses what would break a tabwriter row: a tab opens a new
// column and shifts every row below, a newline splits the row in half. Built
// once — a Replacer compiles a trie, and this runs per column per finding.
var cellReplacer = strings.NewReplacer("\t", " ", "\r", "", "\n", " ")

// cell makes arbitrary text safe to put in a tabwriter column. Reachable text:
// an exception message is somebody's traceback, and findingDetail's default
// branch passes a message through whole.
func cell(s string) string { return cellReplacer.Replace(s) }

// renderTracebacks prints each import error's full traceback under the table,
// the way a compiler prints the source line under its summary.
//
// The table row cannot carry it: a row is one line, and a traceback is not. But
// the table alone was not enough either — a row shows the exception, which says
// WHAT broke, and the frames say WHERE, which is the half you act on. Both
// belong in text output; --output json was carrying the whole message all along
// and is unchanged.
// maxTracebacks caps how many frame blocks print per command run.
//
// One missing dependency makes EVERY dag file an import error with the same
// traceback, so an uncapped dump turned a 50-dag project's report into several
// hundred identical lines with the verdict scrolled off the top. The budget is
// per RUN rather than per table because renderFindingsTable is shared: a
// per-table cap would let --target mwaa,composer print five blocks, a
// suppression count, then five more and a second count.
//
// Only the frames are capped. The table still lists every finding, and
// --output json carries every traceback regardless.
const maxTracebacks = 5

// renderTracebacks prints each import error's frames under the table, the way a
// compiler prints the source line under its summary, drawing from a budget the
// caller owns for the whole run.
func renderTracebacks(w io.Writer, findings []checks.Finding, budget *int) error {
	suppressed := 0
	for i := range findings {
		f := &findings[i]
		// Trimmed before the test, not just inside the loop below: a message
		// with one trailing newline has nothing to show under the table, and
		// printing it anyway produced a "traceback" block that was a verbatim
		// copy of the row above it.
		body := strings.TrimRight(f.Message, "\n\r \t")
		if f.Kind != checks.KindImportError || !strings.Contains(body, "\n") {
			continue
		}
		if *budget == 0 {
			suppressed++
			continue
		}
		if _, err := fmt.Fprintf(w, "\n%s:\n", cell(findingLocation(*f))); err != nil {
			return err
		}
		for _, line := range strings.Split(body, "\n") {
			if _, err := fmt.Fprintf(w, "  %s\n", strings.TrimRight(line, "\r")); err != nil {
				return err
			}
		}
		*budget--
	}
	if suppressed > 0 {
		// "traceback(s)", not "import error(s)": every one of them is still in
		// the table above, and saying otherwise tells the reader the table they
		// are looking at is incomplete.
		if _, err := fmt.Fprintf(w, "\n%d more traceback(s) not shown; --output json carries every one\n", suppressed); err != nil {
			return err
		}
	}
	return nil
}

func findingLocation(f checks.Finding) string {
	if f.Key != "" {
		return envFindingLocation(f)
	}
	if f.DagID != "" {
		return f.DagID
	}
	return f.File
}

func findingDetail(f checks.Finding) string {
	switch f.Kind {
	case checks.KindImportError:
		return exceptionLine(f.Message)
	case checks.KindDuplicateDagID:
		detail := "defined in " + strings.Join(f.Files, ", ")
		// Which copy is live, when Airflow told us. Knowing a dag_id is
		// duplicated without knowing which definition won leaves the reader
		// unable to tell which file to delete.
		if f.File != "" {
			detail += "; Airflow loaded " + f.File
		}
		return detail
	case checks.KindSlowParse:
		return fmt.Sprintf("parsed in %.1fs (threshold %.0fs)", f.ParseSeconds, f.ThresholdSeconds)
	case checks.KindChecksIncomplete:
		// The whole finding is its message: it names checks that did not run,
		// so there is no file or dag_id to point at and the LOCATION column
		// stays empty.
		return f.Message
	case checks.KindEnvMissing, checks.KindEnvInvalid:
		// The declaration is the LOCATION; the message is what is wrong with
		// its value.
		return f.Message
	default:
		return f.Message
	}
}

// firstLine keeps a one-line message to one line. It suits a value that is
// already a sentence — a DAG warning — and NOT a traceback; see exceptionLine.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// exceptionLine reduces a Python traceback to the one line worth putting in a
// table cell or a summary row: the exception type and its message.
//
// Finding it is not "the last line", which is what this did first and is wrong
// for a whole class of ordinary Airflow errors. Python's exception line closes
// the last frame block, and anything printed AFTER it is a continuation — which
// several widely used libraries add:
//
//	sqlalchemy.exc.OperationalError: (psycopg2.OperationalError) could not connect
//	(Background on this error at: https://sqlalche.me/e/20/e3q8)
//
// SQLAlchemy appends that note every time, and a DAG that touches a Connection
// at parse time is ordinary, so the most common database import error showed a
// documentation URL where the exception belonged. pydantic ends with a "For
// further information visit …" line, and an ExceptionGroup ends in a rule of
// dashes. Taking the last line reported those as the diagnosis — worse than the
// banner it replaced, which was useless but never misleading. Worst in
// `af health`, which prints one line per import error and no frames, so the
// exception was not merely buried there but absent.
//
// So the line is found by what it IS rather than by where it sits. A Python
// exception line is a dotted type name followed by a colon —
// "ModuleNotFoundError:", "sqlalchemy.exc.OperationalError:", "ExceptionGroup:"
// — and the last such line is the exception being reported. Nothing else in a
// traceback takes that shape: a frame is `File "x.py", line 2, in <module>`, a
// source echo is a statement, the banner has a space before its colon, and
// SQLAlchemy's note and pydantic's URL both start with something other than an
// identifier.
//
// Position was tried first and is not good enough. "The last unindented line
// whose predecessor is indented" reads the exception correctly for a plain
// traceback but returns:
//
//   - "db_url" — a bare field name — for a pydantic error with TWO invalid
//     fields, because the second field name follows the first field's indented
//     detail block;
//   - the banner for an ExceptionGroup, because Python 3.11+ prefixes every
//     line of one with a "  | " margin, so no line is unindented;
//   - the banner whenever a blank line separates the last frame from the
//     exception, because the predecessor is then empty rather than indented.
//
// The margin is why gutter is stripped before matching: inside an
// ExceptionGroup the exception line is "  | ExceptionGroup: eg (1
// sub-exception)". Indented lines that have NO gutter are skipped, so an echoed
// source line carrying an annotation ("    x: int = 1") cannot be mistaken for
// an exception.
//
// When nothing matches, the message is not a traceback — an import error that
// arrived without one, or Airflow's own "SyntaxError\n  line 3" shape, where
// the type name has no colon after it. Then the FIRST non-empty line is the
// headline; taking the last would return the indented detail under it.
func exceptionLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n\r \t"), "\n")
	if header, ok := multiGroupHeader(lines); ok {
		return header
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], "\r")
		body, gutter := stripGutter(line)
		if !gutter && isIndented(line) {
			continue
		}
		if exceptionTypeRe.MatchString(body) {
			return body
		}
	}
	// Nothing carried a colon. In a rendered traceback that means an exception
	// with no message, which Python prints as the bare type on a line of its
	// own — "AssertionError" after the frames — so the answer is the last
	// unindented line rather than the first.
	//
	// Taking the first returned "Traceback (most recent call last):", the
	// banner, which is the one thing this must never report and which the
	// generated suite fails on by name.
	//
	// Only when a banner is present, because only then is the shape known.
	// Without one this is not a traceback but some other multi-line text, and
	// there the first line is the headline — Airflow's own "SyntaxError\n  line
	// 3" is that shape, and so is any message a caller passes through.
	if hasTracebackBanner(lines) {
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimRight(lines[i], "\r")
			body, gutter := stripGutter(line)
			if !gutter && isIndented(line) {
				continue
			}
			if trimmed := strings.TrimSpace(body); trimmed != "" && !strings.HasPrefix(trimmed, tracebackHeader) {
				return trimmed
			}
		}
	}
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// exceptionTypeRe matches a Python exception line: a dotted type name, then a
// colon, then a space or the end of the line. Anchored, so it cannot match a
// colon later in a sentence.
//
// The space is what keeps a URL out. "https://example.com/errors#e123" is a
// dotted name followed by a colon too, and this scan runs bottom-up, so a
// message whose last line is a bare URL had the URL reported as the exception
// that stopped the run — which is a shape libraries write, and exactly the one
// a reader needs the real name for. Python always prints "Type: message" with
// the space, and an exception carrying no message prints its type with no
// colon at all, which the first-non-empty-line fallback below already covers.
var exceptionTypeRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*:(\s|$)`)

// groupHeaderRe matches the line that opens an ExceptionGroup's own block, by
// the count Python's traceback module appends: "ExceptionGroup: eg (2
// sub-exceptions)". Keyed on the suffix rather than the class name, because the
// name is the author's — BaseExceptionGroup, or any subclass — while the count
// is generated.
var groupHeaderRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*:.*\((\d+) sub-exceptions?\)$`)

// multiGroupHeader returns the header of an ExceptionGroup carrying more than
// one sub-exception.
//
// A group of ONE reports that one: "ExceptionGroup: eg (1 sub-exception)" names
// a container and "ValueError: 1" names what broke, which on `af health` — one
// line, no frames — is the whole diagnosis.
//
// That reasoning stops working as soon as there are several. The scan below
// returns the LAST exception line, which for a group of three is the third,
// chosen for being last rather than for being the problem: it hides that the
// other two happened, and picking any one of them would. So the header is
// reported instead, because the count is the true summary and the frames
// underneath carry the rest.
//
// The FIRST header wins, not the last: groups nest, and the outermost one
// describes the whole failure while an inner one describes a part of it.
//
// A header only counts once Python has announced the block it belongs to. The
// suffix alone is not proof: an ordinary exception whose message happens to end
// "(3 sub-exceptions)" matches it, and in a chained traceback that line is a
// HANDLED exception several frames above the one that actually killed the run —
// so trusting the suffix by itself reports the wrong failure entirely.
func multiGroupHeader(lines []string) (string, bool) {
	// Only the last link of a chain. A group that was CAUGHT, with something
	// else raised while handling it, is not what killed the run — reporting it
	// would name a handled exception several frames above the real one, which
	// is the very thing #184 set out to stop.
	lines = lines[lastChainStart(lines):]

	inGroup := false
	for _, line := range lines {
		body, _ := stripGutter(strings.TrimRight(line, "\r"))
		if !inGroup {
			inGroup = strings.Contains(body, groupBanner)
			continue
		}
		m := groupHeaderRe.FindStringSubmatch(body)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n > 1 {
			return body, true
		}
		// A group of one does not summarize anything its sub-exception does not
		// say better — but keep reading, because that sub-exception can itself
		// be a group of several, and then the inner header is the summary.
	}
	return "", false
}

// lastChainStart returns the index where the final link of a chained traceback
// begins: everything above the last "during handling" or "direct cause" line is
// an exception that was already dealt with.
//
// A separator carrying a gutter is skipped, because it is inside a group block
// describing one sub-exception's own chain rather than ending the outer one.
// Two independent signals, and the later one wins: the separator sentences, and
// the top-level "Traceback (most recent call last):" that opens every plain
// link. Either alone finds the final link of an ordinary chain.
//
// The redundancy is deliberate, and this is the one place in this file that
// needs it. Every other thing matched here degrades to an older, defensible
// answer if Python rewords it — a leaf sub-exception, or the traceback banner.
// This one degrades to reporting an exception that was HANDLED, which is not a
// worse summary but a wrong one, so it does not rest on a single sentence.
//
// They fail independently: the sentences are prose, added by PEP 3134, while
// "Traceback (most recent call last):" is the most entrenched line Python emits.
// Taking the later position is also what makes the pair correct rather than just
// redundant — a chain whose FINAL link is a group has no bare traceback header
// of its own, so the sentence is the only signal there, and a chain whose
// earlier link is a plain traceback would otherwise be re-included by it.
func lastChainStart(lines []string) int {
	start := 0
	for i, line := range lines {
		raw := strings.TrimRight(line, "\r")
		body, gutter := stripGutter(raw)
		if gutter {
			continue
		}
		for _, sep := range chainSeparators {
			if strings.Contains(body, sep) {
				start = max(start, i+1)
				break
			}
		}
		// A link's own header, at column 0: the group banner is indented and
		// carries a gutter, so it is not one of these.
		if !isIndented(raw) && strings.HasPrefix(body, tracebackHeader) {
			start = max(start, i)
		}
	}
	return start
}

// hasTracebackBanner reports whether these lines are a rendered traceback
// rather than some other multi-line text, which decides where its exception
// line is: last in a traceback, first in anything else.
func hasTracebackBanner(lines []string) bool {
	for _, raw := range lines {
		body, _ := stripGutter(strings.TrimRight(raw, "\r"))
		if strings.HasPrefix(strings.TrimSpace(body), tracebackHeader) {
			return true
		}
	}
	return false
}

// tracebackHeader opens each plain link of a traceback.
const tracebackHeader = "Traceback (most recent call last):"

// chainSeparators are the two sentences Python puts between the links of a
// chained traceback, oldest first.
var chainSeparators = []string{
	"During handling of the above exception, another exception occurred:",
	"The above exception was the direct cause of the following exception:",
}

// groupBanner opens an ExceptionGroup's block in Python's rendering, on the
// line above its header.
const groupBanner = "Exception Group Traceback"

// stripGutter removes the margin Python 3.11+ draws down the left of an
// ExceptionGroup traceback ("  | ", "  +-+--- 1 ---"), reporting whether one
// was there. A line with a gutter is a traceback line however deeply it is
// indented; a line without one is judged on its own indentation.
func stripGutter(line string) (body string, gutter bool) {
	t := strings.TrimLeft(line, " \t")
	if t == "" || (t[0] != '|' && t[0] != '+') {
		return strings.TrimSpace(line), false
	}
	return strings.TrimSpace(strings.TrimLeft(t[1:], " \t-+|")), true
}

// isIndented reports whether a line is indented at all.
func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func checkSummaryLine(s checkSummary) string {
	verdict := "passed"
	if !s.Passed {
		verdict = verdictFailed
	}
	tail := ""
	if s.Strict {
		tail = " (strict)"
	}
	return fmt.Sprintf("checks %s%s: %d DAGs, %d errors, %d warnings", verdict, tail, s.Dags, s.Errors, s.Warnings)
}

// blocked reports an outcome where check reached no verdict at all — not in a
// project, a manifest that will not load, an unusable environment, an unknown
// target — and exits 2.
//
// One door for all of them, because the three contracts they share are easy to
// break one site at a time: the diagnosis goes to stdout (docs/install.md tells
// an agent to read it), json mode gets {"event":"error"} rather than cobra's
// generic object, and the code is 2 rather than 1. Exit 1 means the DAGs failed
// a check, so a CI job branching on the two must never see it for "you are not
// in a project directory".
func blocked(r Renderer, err error) error {
	if rerr := renderCheckBlocked(r, err); rerr != nil {
		return rerr
	}
	return &ExitError{Code: checks.ExitEnvNotReady}
}

// checkBlocked is the line `astro local check` publishes when it cannot run
// at all. Named rather than anonymous so the schema pins can hold it.
type checkBlocked struct {
	Event   string `json:"event"`
	Message string `json:"message"`
}

// renderCheckBlocked writes the reason check could not reach a verdict. In
// json mode it is a single structured line; in text mode, the guidance.
func renderCheckBlocked(r Renderer, err error) error {
	msg := err.Error()
	return r.Emit(checkBlocked{Event: "error", Message: msg}, func(w io.Writer) error {
		_, werr := fmt.Fprintln(w, msg)
		return werr
	})
}

// renderTargetChecks writes one block per target. In json mode each target is
// one NDJSON line (the TargetReport, findings array and all); in text mode each
// is a headed block with the version mapping, a findings table, any constraints
// result, and a one-line verdict.
func renderTargetChecks(r Renderer, reports []checks.TargetReport, strict bool) error {
	if r.Format == FormatJSON {
		for i := range reports {
			if err := r.Emit(reports[i], nil); err != nil {
				return err
			}
		}
		return nil
	}
	// One budget across every target, so the same traceback is not dumped once
	// per report.
	budget := maxTracebacks
	for i := range reports {
		if i > 0 {
			if _, err := fmt.Fprintln(r.Out); err != nil {
				return err
			}
		}
		if err := renderTargetReport(r.Out, &reports[i], strict, &budget); err != nil {
			return err
		}
	}
	return nil
}

func renderTargetReport(w io.Writer, rep *checks.TargetReport, strict bool, budget *int) error {
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
	} else {
		if err := renderFindingsTable(w, rep.Findings, budget); err != nil {
			return err
		}
		// The same separator renderCheck emits. Both callers of
		// renderFindingsTable need it, and owning it here rather than inside
		// the findings block is what stops the spacing depending on which kind
		// of finding printed last.
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
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
	case c.Conflicted():
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
		verdict = verdictFailed
	}
	tail := ""
	if strict {
		tail = " (strict)"
	}
	why := ""
	if rep.Constraints.Conflicted() {
		why = "dependencies conflict with MWAA's constraints; "
	}
	return fmt.Sprintf("check %s%s for %s: %s%d DAGs, %d errors, %d warnings", verdict, tail, rep.Target, why, rep.DagCount, rep.Errors, rep.Warnings)
}

// verdictFailed is a check's verdict when it did not pass.
const verdictFailed = "failed"
