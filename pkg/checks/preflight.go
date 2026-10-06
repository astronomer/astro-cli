package checks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/platformversions"
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
// Astro is a known target and is NOT checked by Preflight: it has no platform
// version table to check a pin against, so Run is what validates it. A caller
// routing on this has to send TargetAstro to Run and the rest to Preflight,
// which is what cmd/local/check.go does. Preflight refuses TargetAstro by name
// rather than reporting it as unknown.
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
// failure: the pre-flight notes it and the check stands on its DAG findings
// alone.
var ErrConstraintsUnavailable = errors.New("could not fetch the platform constraints file")

// ConstraintConflict reports that the project's dependencies do not resolve
// under the platform's constraints file. It fails the check: the same conflict
// would fail a slow MWAA environment update. Summary is the resolver's
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
	// Python is the interpreter to provision: a concrete version ("3.12"), or
	// a request uv resolves (">=3.10,<3.13", a project's requires-python).
	// Empty lets the provisioner pick one.
	//
	// A target check passes the platform's version, because the platform
	// decides. A provisioned project check passes the manifest's
	// requires-python, because the venv lives outside the project and uv
	// cannot read the constraint from the manifest itself.
	Python string
	// Reqs is the requirement set to install, including the pinned
	// apache-airflow line.
	Reqs []string
	// Constraints limits the versions the install may pick without requiring
	// anything: the project's [tool.uv] constraint-dependencies, which uv
	// applies to the project's own environment but cannot see from a scratch
	// one.
	Constraints []string
	// FindLinks are the per-package index pages the project's
	// [tool.uv.sources] name (manifest.UV.IndexPages), which the scratch
	// environment cannot read from the project: the pages Astronomer's build
	// of Airflow is found on.
	FindLinks []string
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
	// Constraints is the manifest's [tool.uv] constraint-dependencies.
	Constraints []string
	// Env is the environment the DAGs are imported under; see ParseInput.Env.
	Env []string
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

// Conflicted reports whether the dependencies were resolved against the
// constraints file and failed to solve. A skip is not a conflict.
func (c *ConstraintOutcome) Conflicted() bool {
	return c != nil && c.Checked && !c.OK
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
// operational failure is ExitEnvNotReady, findings or a constraints conflict
// are ExitChecksFailed, clean is ExitOK.
func (r *TargetReport) ExitCode(strict bool) int {
	if r.OpError != "" {
		return ExitEnvNotReady
	}
	if r.Errors > 0 || (strict && r.Warnings > 0) || r.Constraints.Conflicted() {
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
//
//nolint:gocritic // hugeParam: PreflightInput is a contract struct, passed by value like the core ones (docs/architecture.md).
func Preflight(ctx context.Context, target string, in PreflightInput, prov Provisioner, parser TargetParser, strict bool, progress func(string)) TargetReport {
	// A consumer with nowhere to stream notes passes nil, and this function
	// promises never to return an error — so it must not panic on one either.
	// Astro Desktop is the first such caller: it has no text renderer.
	if progress == nil {
		progress = func(string) {}
	}
	rep := TargetReport{Target: target}

	// Named separately from an unknown target: astro IS known (KnownTarget says
	// so) and is checked by Run instead, so reporting it as unknown would send
	// a caller looking for a typo that is not there.
	if target == TargetAstro {
		return TargetReport{Target: target, OpError: fmt.Sprintf("target %q is checked by Run, not Preflight: it has no platform version table to check a pin against", target)}
	}
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

	pythonBin, err := prov.EnsureVenv(ctx, VenvSpec{Airflow: match.Airflow, Python: python, Reqs: reqs, Constraints: sortedCopy(in.Constraints)}, progress)
	if err != nil {
		rep.OpError = fmt.Sprintf("building the %s check environment: %v", target, err)
		return rep
	}

	report, err := parser.ParseWith(ctx, pythonBin, ParseInput{ProjectPath: in.ProjectPath, DagsDir: in.DagsDir, Env: in.Env})
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

// sortedCopy orders a list for a cache key that does not depend on how the
// manifest happened to order it, without reordering the caller's slice.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// dropAirflow removes the requirements that state the Airflow version,
// apache-airflow and apache-airflow-core, from a dependency list; the caller
// adds the platform's own version back. Keeping a core pin beside it would ask
// for two Airflows, which no resolve satisfies. The providers and the task SDK
// stay.
func dropAirflow(deps []string) []string {
	return manifest.WithoutAirflow(deps)
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
