package scaffold

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// Import turns a plain-Airflow repo (a dags/ folder and, usually, a
// requirements.txt) into an Astro project. It is deterministic and offline:
// it detects the shape, never guesses, and writes to a fresh target directory
// so the source repo is never touched. Dags are copied, not linked, so the new
// project stands on its own. A failed uv lock is reported, not rolled back.

// Refusal sentinels for Import, wrapped with context. Callers branch with
// errors.Is. ErrV1Project and ErrManifestExists (from the greenfield path) also
// apply, to the source and the target respectively.
var (
	// ErrNotAirflowRepo reports a source with nothing to import: no dags/
	// directory and no DAG .py files at its root.
	ErrNotAirflowRepo = errors.New("not a plain-Airflow repo")
	// ErrSourceIsProject reports a source that already carries a
	// pyproject.toml — an existing project, not a plain-Airflow repo.
	ErrSourceIsProject = errors.New("source already has a pyproject.toml")
	// ErrImportInPlace reports a target that is the source, or inside it:
	// the import writes to a new directory and leaves the source untouched.
	ErrImportInPlace = errors.New("import writes to a new directory, not into the source")
)

// Locker resolves a scaffolded project's dependencies with uv. *uv.Client
// satisfies it; tests pass a stub. Import treats a nil Locker as "skip the
// lock" and says so in the report.
type Locker interface {
	Lock(ctx context.Context, project string, stdio uv.Stdio) error
}

// ImportOptions adjust what Import does.
type ImportOptions struct {
	// Name is the [project] name. Empty derives it from the target directory.
	Name string
	// AirflowVersion overrides the pin detection: when set it wins over both
	// a requirements pin and the default.
	AirflowVersion string
	// GOOS overrides runtime.GOOS, so tests can check the Windows layout.
	GOOS string
	// Lock resolves the scaffolded project. Nil skips the lock.
	Lock Locker
	// LockOutput receives uv's live output during the lock; nil discards it.
	// The CLI passes stdout in text mode and nothing in json mode.
	LockOutput io.Writer
}

// ImportResult reports what Import did. It is the `astro init --from` output
// payload in both text and json mode.
type ImportResult struct {
	Source string `json:"source"`
	Dir    string `json:"dir"`
	Name   string `json:"name"`
	// Airflow is the pinned version; AirflowFrom is "requirements", "flag",
	// or "default", so the report can explain where it came from.
	Airflow     string `json:"airflow"`
	AirflowFrom string `json:"airflowFrom"`
	// Created and Skipped list the scaffold entries, as for greenfield init.
	Created []string `json:"created"`
	Skipped []string `json:"skipped,omitempty"`
	// Dags and Plugins count the files copied from the source.
	Dags    int `json:"dags"`
	Plugins int `json:"plugins,omitempty"`
	// Dependencies counts the entries written to [project.dependencies]: the
	// requirements carried from the source, plus the apache-airflow line the
	// import adds when the source named none.
	Dependencies int `json:"dependencies"`
	// Warnings collects non-fatal notes: carried lines, a missing
	// requirements.txt, a defaulted Airflow version.
	Warnings []string `json:"warnings,omitempty"`
	// Lock reports the uv lock attempt.
	Lock LockReport `json:"lock"`
}

// LockReport reports the uv lock attempt. A failure names what it could,
// pulled from uv's typed ResolutionError.
type LockReport struct {
	Attempted bool `json:"attempted"`
	Locked    bool `json:"locked"`
	// Packages and Constraints name the conflict on a resolution failure.
	Packages    []string `json:"packages,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	// Summary is uv's one-line explanation; Error is the full error text.
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Import scaffolds an Astro project in dst from the plain-Airflow repo at src.
// It never writes to src. A refusal (bad source, unsafe target) returns an
// error and writes nothing; a scaffold that lands but fails to lock returns
// the result with the failure in Lock, not an error.
//
//nolint:gocritic // ImportOptions is a by-value options struct, like scaffold.Run's Options.
func Import(ctx context.Context, src, dst string, opts ImportOptions) (*ImportResult, error) {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("resolving source %s: %w", src, err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return nil, fmt.Errorf("resolving target %s: %w", dst, err)
	}
	if within(absDst, absSrc) {
		return nil, fmt.Errorf("%s: %w; pass a new target, e.g. astro init <dir> --from %s", absDst, ErrImportInPlace, src)
	}

	shape, err := detect(absSrc)
	if err != nil {
		return nil, err
	}
	// The target must be safe to scaffold, same rule as greenfield init.
	if err := refuse(absDst); err != nil {
		return nil, err
	}

	reqs, haveReqs := readRequirements(absSrc)

	name := opts.Name
	if name == "" {
		name = deriveName(absDst)
	}
	version, from := resolveAirflow(opts.AirflowVersion, reqs)

	// Guarantee the project can start: when the source carries no
	// apache-airflow requirement, add one matching the resolved pin, so
	// [project.dependencies] installs Airflow just as a greenfield init does.
	// A source that already names apache-airflow is left verbatim — rewriting
	// it would be guessing.
	deps := reqs
	addedAirflow := !hasAirflowDependency(reqs)
	if addedAirflow {
		deps = append(slices.Clone(reqs), reqLine{kind: reqDependency, text: airflowRequirement(version)})
	}
	depsLiteral, depCount := renderDependencies(deps)

	pyproject, err := renderManifest(name, version, depsLiteral)
	if err != nil {
		return nil, err
	}

	res := &ImportResult{
		Source:       absSrc,
		Dir:          absDst,
		Name:         name,
		Airflow:      version,
		AirflowFrom:  from,
		Dependencies: depCount,
	}
	res.Warnings = append(res.Warnings, carriedWarnings(reqs)...)
	if !haveReqs {
		res.Warnings = append(res.Warnings, "no requirements.txt in the source")
	}
	if from == "default" {
		res.Warnings = append(res.Warnings, "no apache-airflow pin in requirements.txt; using the default Airflow "+DefaultAirflowVersion)
	}
	if addedAirflow {
		res.Warnings = append(res.Warnings, "added "+airflowRequirement(version)+" to dependencies so the project's environment includes Airflow")
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if err := write(absDst, pyproject, goos != "windows", &res.Created, &res.Skipped); err != nil {
		return nil, err
	}

	dags, err := copyDags(shape, filepath.Join(absDst, dirDags))
	if err != nil {
		return nil, err
	}
	res.Dags = dags
	plugins, err := copyIfDir(filepath.Join(absSrc, dirPlugins), filepath.Join(absDst, dirPlugins))
	if err != nil {
		return nil, err
	}
	res.Plugins = plugins

	runLock(ctx, absDst, opts.Lock, opts.LockOutput, res)
	return res, nil
}

// sourceShape is what detect found in the source: either a dags/ directory to
// copy wholesale, or DAG .py files at the source root.
type sourceShape struct {
	dagsDir    string   // set when the source has a dags/ directory
	rootDagPys []string // set when DAGs are .py files at the source root
}

// detect reads the source and returns its shape, refusing a source that is
// already a project, a v1 astro project, or not an Airflow repo at all.
func detect(src string) (sourceShape, error) {
	info, err := os.Stat(src)
	if err != nil {
		return sourceShape{}, fmt.Errorf("reading source %s: %w", src, err)
	}
	if !info.IsDir() {
		return sourceShape{}, fmt.Errorf("source %s is not a directory", src)
	}
	if _, err := os.Stat(filepath.Join(src, project.Marker)); err == nil {
		return sourceShape{}, fmt.Errorf("%s %w; use `astro init` in place or edit that manifest", src, ErrSourceIsProject)
	}
	if _, err := os.Stat(filepath.Join(src, "Dockerfile")); err == nil {
		if fi, err := os.Stat(filepath.Join(src, ".astro")); err == nil && fi.IsDir() {
			return sourceShape{}, fmt.Errorf("%s %w (Dockerfile and .astro/); v1 migration ships later — use astro CLI 1.x with this project for now", src, ErrV1Project)
		}
	}

	var shape sourceShape
	if fi, err := os.Stat(filepath.Join(src, dirDags)); err == nil && fi.IsDir() {
		shape.dagsDir = filepath.Join(src, dirDags)
		return shape, nil
	}
	pys, err := rootDagFiles(src)
	if err != nil {
		return sourceShape{}, err
	}
	if len(pys) == 0 {
		return sourceShape{}, fmt.Errorf("%s: %w (no dags/ directory and no .py files at its root)", src, ErrNotAirflowRepo)
	}
	shape.rootDagPys = pys
	return shape, nil
}

// rootDagFiles lists the .py files directly under src (not recursing): the
// "DAGs at the repo root" shape. It detects .py presence and no more — it does
// not parse them to confirm they define DAGs.
func rootDagFiles(src string) ([]string, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("reading source %s: %w", src, err)
	}
	var pys []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
			pys = append(pys, filepath.Join(src, e.Name()))
		}
	}
	return pys, nil
}

// copyDags copies the source DAGs into dst (the target dags/ directory),
// which write already created. It returns the number of files copied.
func copyDags(shape sourceShape, dst string) (int, error) {
	if shape.dagsDir != "" {
		return copyTree(shape.dagsDir, dst)
	}
	var copied int
	for _, py := range shape.rootDagPys {
		did, err := copyFileInto(py, filepath.Join(dst, filepath.Base(py)))
		if err != nil {
			return copied, err
		}
		if did {
			copied++
		}
	}
	return copied, nil
}

// copyIfDir copies src into dst when src is a directory, returning the number
// of files copied (0 when src is absent).
func copyIfDir(src, dst string) (int, error) {
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return 0, nil
	}
	return copyTree(src, dst)
}

// copyTree copies every file under src into dst, creating directories as it
// goes and never overwriting a file that already exists (the import is not
// destructive). It returns the number of files copied.
func copyTree(src, dst string) (int, error) {
	var copied int
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, sockets, devices
		}
		did, err := copyFileInto(path, target)
		if err != nil {
			return err
		}
		if did {
			copied++
		}
		return nil
	})
	return copied, err
}

// copyFileInto copies src to dst unless dst already exists, reporting whether
// it wrote. The parent directory must already exist.
func copyFileInto(src, dst string) (bool, error) {
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", src, err)
	}
	//nolint:gosec // G306: see filePerm
	if err := os.WriteFile(dst, data, filePerm); err != nil {
		return false, fmt.Errorf("writing %s: %w", dst, err)
	}
	return true, nil
}

// readRequirements reads the source requirements.txt, returning the parsed
// lines and whether the file was there.
func readRequirements(src string) (lines []reqLine, present bool) {
	data, err := os.ReadFile(filepath.Join(src, "requirements.txt"))
	if err != nil {
		return nil, false
	}
	return parseRequirements(data), true
}

// resolveAirflow decides the pin and where it came from: an explicit flag
// wins, then a clean requirements pin, then the default.
func resolveAirflow(flag string, reqs []reqLine) (version, from string) {
	if flag != "" {
		return flag, "flag"
	}
	if v, ok := airflowPin(reqs); ok {
		return v, "requirements"
	}
	return DefaultAirflowVersion, "default"
}

// runLock attempts the uv lock and records the outcome. A resolution failure
// is a report, not an error: the project stays scaffolded so the user can fix
// the conflict in pyproject.toml.
func runLock(ctx context.Context, dst string, lock Locker, out io.Writer, res *ImportResult) {
	if lock == nil {
		res.Warnings = append(res.Warnings, "skipped uv lock (uv is not available)")
		return
	}
	res.Lock.Attempted = true
	stdio := uv.Stdio{}
	if out != nil {
		stdio.Out = out
		stdio.Err = out
	}
	err := lock.Lock(ctx, dst, stdio)
	if err == nil {
		res.Lock.Locked = true
		return
	}
	var re *uv.ResolutionError
	if errors.As(err, &re) {
		res.Lock.Packages = re.Packages
		res.Lock.Constraints = re.Constraints
		res.Lock.Summary = re.Summary
	}
	res.Lock.Error = err.Error()
}

// within reports whether path is dir or sits inside it, so the import can
// refuse a target that would land in the source.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
