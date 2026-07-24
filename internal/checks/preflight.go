package checks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/internal/platformversions"
)

// Target names a platform a pre-flight check can run against. astro is the
// project's own Airflow (the default `astro local check`); mwaa and composer
// map the manifest pin to the version that platform actually runs and check
// against it.
const (
	TargetAstro    = "astro"
	TargetMWAA     = "mwaa"
	TargetComposer = "composer"
)

// KnownTarget reports whether name is a target this package checks.
func KnownTarget(name string) bool {
	switch name {
	case TargetAstro, TargetMWAA, TargetComposer:
		return true
	default:
		return false
	}
}

// ErrConstraintsUnavailable reports that the platform's constraints file could
// not be fetched — offline, or the URL did not answer. It is a skip, not a
// failure: the pre-flight notes it and the check still passes on its DAG
// findings alone.
var ErrConstraintsUnavailable = errors.New("could not fetch the platform constraints file")

// ConstraintConflict reports that the project's dependencies do not resolve
// under the platform's constraints file. It is a real finding: the same
// conflict would fail a slow MWAA environment update. Summary is the resolver's
// one-line explanation; Detail carries its full output.
type ConstraintConflict struct {
	Summary string
	Detail  string
}

func (e *ConstraintConflict) Error() string { return e.Summary }

// VenvSpec describes the scratch environment a target check needs: an
// interpreter version (empty lets the provisioner choose) and the full
// requirement set, the platform's Airflow pin already folded in.
type VenvSpec struct {
	// Airflow is the platform's Airflow version, for a readable cache-directory
	// name and progress text. The pin is also present in Reqs, so the cache key
	// need not read it separately.
	Airflow string
	// Python is the interpreter version to provision, e.g. "3.12". Empty lets
	// the provisioner pick one.
	Python string
	// Reqs is the requirement set to install, including the pinned
	// apache-airflow line.
	Reqs []string
}

// Provisioner builds and caches a scratch venv for a target, and resolves a
// requirement set against a platform constraints file. It is the seam the cmd
// layer fills with uv; tests fill it with a fake so no real install runs.
type Provisioner interface {
	// EnsureVenv provisions (or reuses from cache) a venv matching spec and
	// returns its Python interpreter path. progress receives human notes as
	// the work happens ("provisioning…", "reusing cached…").
	EnsureVenv(ctx context.Context, spec VenvSpec, progress func(string)) (pythonBin string, err error)
	// ResolveConstraints resolves reqs against the constraints file at url,
	// without installing anything. It returns nil on a clean resolve, a
	// *ConstraintConflict on a solver conflict, ErrConstraintsUnavailable when
	// the file could not be fetched, and any other error for an operational
	// failure.
	ResolveConstraints(ctx context.Context, reqs []string, url, pythonVersion string) error
}

// TargetParser runs the embedded DAG parse against an explicit interpreter —
// the scratch venv the Provisioner built, not the project's own .venv.
// *VenvRunner satisfies it.
type TargetParser interface {
	ParseWith(ctx context.Context, python string, in ParseInput) (ParseReport, error)
}

// PreflightInput is what a target check needs about the project.
type PreflightInput struct {
	ProjectPath string
	DagsDir     string
	// Pin is the manifest's Airflow pin (may be partial, e.g. "3.1").
	Pin string
	// Deps is the manifest's [project].dependencies.
	Deps []string
}

// ConstraintOutcome is the MWAA constraints pre-flight result. Exactly one of
// the states holds: Checked+OK, Checked+Conflict, or Skipped.
type ConstraintOutcome struct {
	URL      string `json:"url"`
	Checked  bool   `json:"checked"`
	OK       bool   `json:"ok"`
	Conflict string `json:"conflict,omitempty"`
	Skipped  string `json:"skipped,omitempty"`
}

// TargetReport is one target's pre-flight outcome, rendered by the cmd layer.
// It carries the DAG findings plus the version mapping and any constraints
// result, so the reader sees what was checked and against which version.
type TargetReport struct {
	Target string `json:"target"`
	// AirflowChecked is the version the check actually ran against.
	AirflowChecked string `json:"airflow_checked"`
	// MappedFrom is the manifest pin, set only when the check mapped it down to
	// a lower supported version.
	MappedFrom string `json:"mapped_from,omitempty"`
	// Notes carries the honesty lines: the version-mapping note, any Python
	// skew caveat.
	Notes []string `json:"notes,omitempty"`
	// Constraints is the MWAA constraints resolution outcome; nil for other
	// targets.
	Constraints *ConstraintOutcome `json:"constraints,omitempty"`

	Findings []Finding `json:"findings"`
	DagCount int       `json:"dags"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	// OpError names an operational failure that stopped the check for this
	// target (no version maps, the venv would not build). When set, the check
	// reached no verdict.
	OpError string `json:"error,omitempty"`
}

// ExitCode maps a report to a process exit code, strict included: an
// operational failure is ExitEnvNotReady, findings are ExitChecksFailed, clean
// is ExitOK.
func (r *TargetReport) ExitCode(strict bool) int {
	if r.OpError != "" {
		return ExitEnvNotReady
	}
	if r.Errors > 0 || (strict && r.Warnings > 0) {
		return ExitChecksFailed
	}
	return ExitOK
}

// Preflight runs one target's check: map the pin to the version the platform
// runs, provision a scratch venv with that Airflow plus the project's deps,
// parse the DAGs inside it, and — for MWAA — resolve the deps against the
// platform's constraints file. It never returns an error: every failure is
// recorded on the report (OpError for operational ones) so a multi-target run
// reports each target and still picks a single exit code. progress receives
// human notes for the text renderer to stream.
func Preflight(ctx context.Context, target string, in PreflightInput, prov Provisioner, parser TargetParser, strict bool, progress func(string)) TargetReport {
	rep := TargetReport{Target: target}

	supported, ok := supportedVersions(target)
	if !ok {
		rep.OpError = fmt.Sprintf("unknown target %q", target)
		return rep
	}
	if strings.TrimSpace(in.Pin) == "" {
		rep.OpError = "the manifest sets no Airflow version to check against"
		return rep
	}

	match, downgraded, ok := platformversions.Match(in.Pin, supported)
	if !ok {
		rep.OpError = fmt.Sprintf("%s runs no Airflow at or below the manifest pin %q (it offers: %s)",
			target, in.Pin, platformversions.List(supported))
		return rep
	}
	rep.AirflowChecked = match.Airflow
	if downgraded {
		rep.MappedFrom = in.Pin
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"%s does not offer Airflow %s; checking against %s, the closest version it runs — a %s-only feature would break there",
			target, in.Pin, match.Airflow, in.Pin))
	}

	reqs := requirementSet(match.Airflow, in.Deps)

	python := match.Python
	if python == "" {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"%s pins its own Python; checking with the default interpreter, so a Python-version-specific issue may not show", target))
	}

	pythonBin, err := prov.EnsureVenv(ctx, VenvSpec{Airflow: match.Airflow, Python: python, Reqs: reqs}, progress)
	if err != nil {
		rep.OpError = fmt.Sprintf("building the %s check environment: %v", target, err)
		return rep
	}

	report, err := parser.ParseWith(ctx, pythonBin, ParseInput{ProjectPath: in.ProjectPath, DagsDir: in.DagsDir})
	if err != nil {
		rep.OpError = fmt.Sprintf("parsing DAGs for %s: %v", target, err)
		return rep
	}
	if report.Fatal != "" {
		rep.OpError = fmt.Sprintf("%s (in the %s check environment)", report.Fatal, target)
		return rep
	}
	res := evaluate(report)
	rep.Findings = res.Findings
	if rep.Findings == nil {
		rep.Findings = []Finding{}
	}
	rep.DagCount = res.DagCount
	rep.Errors = res.Errors
	rep.Warnings = res.Warnings

	if target == TargetMWAA {
		rep.Constraints = mwaaConstraints(ctx, prov, in.Deps, match, progress)
	}
	return rep
}

// supportedVersions returns the version list for a platform target. astro has
// none — it is checked against the project's own venv, not a mapped version.
func supportedVersions(target string) ([]platformversions.Version, bool) {
	switch target {
	case TargetMWAA:
		return platformversions.MWAA, true
	case TargetComposer:
		return platformversions.Composer, true
	default:
		return nil, false
	}
}

// mwaaConstraints runs the MWAA constraints resolution and shapes its outcome.
// The requirement set drops apache-airflow (MWAA provides it and the constraint
// pins it), mirroring what the mwaa package target writes.
func mwaaConstraints(ctx context.Context, prov Provisioner, deps []string, v platformversions.Version, progress func(string)) *ConstraintOutcome {
	url := platformversions.MWAAConstraintURL(v)
	out := &ConstraintOutcome{URL: url}
	progress(fmt.Sprintf("resolving dependencies against MWAA's constraints for Airflow %s", v.Airflow))

	err := prov.ResolveConstraints(ctx, dropAirflow(deps), url, v.Python)
	switch {
	case err == nil:
		out.Checked = true
		out.OK = true
	case errors.Is(err, ErrConstraintsUnavailable):
		out.Skipped = "could not fetch constraints (network required); skipped the requirements resolution"
	default:
		var conflict *ConstraintConflict
		if errors.As(err, &conflict) {
			out.Checked = true
			out.Conflict = conflict.Summary
		} else {
			out.Skipped = fmt.Sprintf("constraints resolution did not run: %v", err)
		}
	}
	return out
}

// requirementSet builds the install list for a target's scratch venv: the
// project's deps with any apache-airflow pin swapped for the platform's version,
// so the check runs against the Airflow that would actually run. The result is
// sorted so the cache key over it is stable.
func requirementSet(airflow string, deps []string) []string {
	out := append([]string{"apache-airflow==" + airflow}, dropAirflow(deps)...)
	sort.Strings(out)
	return out
}

// dropAirflow removes any apache-airflow pin from a dependency list; the caller
// adds the platform's own version back. It matches the base distribution only,
// leaving apache-airflow-providers-* in place.
func dropAirflow(deps []string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if distName(d) == "apache-airflow" {
			continue
		}
		out = append(out, d)
	}
	return out
}

// distName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
// Mirrors internal/pack.distName and internal/imagebuild.distName; the three
// stay separate rather than couple these packages over one small helper.
func distName(req string) string {
	s := strings.TrimSpace(req)
	if i := strings.IndexAny(s, "[ \t<>=!~;@("); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

// DefaultDagsDir is the dags/ folder under a project root.
func DefaultDagsDir(projectPath string) string {
	return filepath.Join(projectPath, "dags")
}

// VenvInterpreter is the Python executable inside a venv directory. The layout
// differs on Windows, so switch on GOOS rather than build-tagging the file. The
// project-venv parser and the scratch-venv provisioner both resolve their
// interpreter through here, so the Windows split lives in one place.
func VenvInterpreter(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python")
}
