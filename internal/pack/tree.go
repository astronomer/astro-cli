package pack

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// treePerm is the mode a written artifact file and directory carry: owner
// read/write, group/other read. A bucket artifact is not secret, and an
// upload tool needs to read it.
const (
	treeFilePerm = 0o644
	treeDirPerm  = 0o755
)

// airflowDist is the distribution the managed platforms provide themselves, so
// the derived requirements drop it — the same rule the runtime image build
// follows (internal/imagebuild.runtimeDeps).
const airflowDist = "apache-airflow"

// checklistName is the file a tree target writes when the manifest declares
// environment values. MWAA and Composer configure those in their own consoles,
// so the artifact carries the list, not the values.
const checklistName = "ENV_SETUP.md"

// prepareTree validates the request for a tree target and returns a fresh,
// empty artifact directory. It errors on a nil manifest or a project with no
// name (the same guards the astro target keeps), and clears any earlier build
// so a removed dag does not linger.
func prepareTree(req Request, target string) (string, error) {
	if req.Manifest == nil {
		return "", errors.New("no manifest to package")
	}
	if req.Manifest.Project.Name == "" {
		return "", errors.New("the project has no name; set [project] name in pyproject.toml")
	}
	outDir := req.OutDir
	if outDir == "" {
		outDir = filepath.Join(req.ProjectDir, "dist", target)
	}
	if err := os.RemoveAll(outDir); err != nil {
		return "", fmt.Errorf("clearing the artifact directory %s: %w", outDir, err)
	}
	if err := os.MkdirAll(outDir, treeDirPerm); err != nil {
		return "", fmt.Errorf("creating the artifact directory %s: %w", outDir, err)
	}
	return outDir, nil
}

// copyDags copies the project's dags/ into the artifact under dags/. A project
// with no dags/ is unusual but valid — an empty dags/ is created so the bucket
// layout is complete either way.
func copyDags(projectDir, outDir string) error {
	src := filepath.Join(projectDir, "dags")
	dst := filepath.Join(outDir, "dags")
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return os.MkdirAll(dst, treeDirPerm)
	}
	return copyDir(src, dst)
}

// copyDir copies the tree at src to dst, recreating directories and files.
// Everything the user wrote ships as-is; Python's own cache directory does not
// — see skipCache.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if skipCache(d, path, src) {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, treeDirPerm)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks and other irregular entries
		}
		return copyFile(path, target)
	})
}

const pycacheDir = "__pycache__"

// skipCache reports whether a walked directory is Python's bytecode cache, and
// is the ONE rule the three walks over a project share — copyDir, zipDir and
// dirHasFiles. They have to share it: dirHasFiles decides whether MWAA gets a
// plugins.zip at all, and when it answered that question by a different rule
// than the walk it authorizes, a plugins/ holding only a __pycache__ with one
// non-.pyc file in it (Cython's .so, or the temp file CPython leaves when a
// parse is killed mid-write) produced a 22-byte, zero-entry plugins.zip — and
// the artifact's next-steps text still told the user to point a live MWAA
// environment at it. Worse than the junk it replaced, which was at least inert.
//
// It is deliberately a DIRECTORY rule and nothing more. An earlier version also
// dropped any file named *.pyc or *.pyo, which was wrong twice:
//
//   - A loose sourceless .pyc, with no .py beside it, IS importable — that is
//     PEP 3147's legacy layout, and a project may legitimately vendor a
//     compiled-only module. Dropping it silently broke the DAG that imported
//     it. The rationale for dropping it ("Python will not load a sourceless
//     .pyc") is true only INSIDE __pycache__, which this rule already covers,
//     so the suffix check bought nothing and cost that.
//   - .pyo has not been produced by any CPython since 3.5 (PEP 488 replaced it
//     with .opt-N.pyc inside __pycache__), so that arm could only ever have
//     matched a user's own file.
//
// Nor is this the same rule pkg/scaffold applies when it decides whether a
// project has DAGs: that one also treats dotfiles as bookkeeping, and here a
// dotfile must ship, because dags/.airflowignore is a real file Airflow reads
// out of the bucket (pkg/airflowrt scaffolds one). The two look alike and must
// not be merged.
//
// The walk root is never skipped: WalkDir calls back for src itself, and
// returning SkipDir there would silently produce an empty artifact for
// `--out-dir __pycache__`.
func skipCache(d os.DirEntry, path, root string) bool {
	return d.Name() == pycacheDir && path != root
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), treeDirPerm); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, treeFilePerm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// writeFile writes a generated file into the artifact.
func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), treeFilePerm)
}

// dirHasFiles reports whether dir holds at least one regular, non-hidden file.
// A folder carrying only a .gitkeep placeholder reads as empty, so an untouched
// plugins/ or include/ does not produce an empty plugins.zip or a stray warning.
//
// It must answer by the SAME rule the walk it gates uses, which is what
// skipCache is for: this predicate decides whether MWAA is told to upload a
// plugins.zip, and zipDir decides what goes in it. Any disagreement between the
// two is an empty zip with an upload instruction attached.
//
// The doc above has always said "regular", and now the code does too. Without
// that check a plugins/ whose only entry is a symlink answered true here and
// then produced an empty zip in zipDir, which drops irregular entries — the
// same failure as the cache mismatch, by a different route.
func dirHasFiles(dir string) bool {
	found := false
	//nolint:errcheck // a walk error just leaves found false
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipCache(d, path, dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !d.Type().IsRegular() {
			return nil
		}
		found = true
		return filepath.SkipAll
	})
	return found
}

// zipTree writes the whole artifact directory to a .zip at path (the --save
// hand-off), so a CI job can upload one file. It returns the zip's size.
func zipTree(root, path string) (int64, error) {
	if err := zipDir(root, path); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, nil //nolint:nilerr // size is best-effort; the zip is written
	}
	return info.Size(), nil
}

// zipDir writes the tree at root to a zip at path. Entry names are relative to
// root and use forward slashes so the zip is portable.
func zipDir(root, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, treeFilePerm)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipCache(d, p, root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		in.Close()
		return err
	})
	if err != nil {
		zw.Close() //nolint:errcheck // returning the walk error
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// dropAirflow returns the manifest dependencies with the apache-airflow
// distribution removed: every managed platform ships Airflow itself and rejects
// a pin of it in the requirements. This mirrors internal/imagebuild.runtimeDeps,
// which does the same for the runtime image; the copy lives here because that
// helper (and distName) are unexported.
func dropAirflow(deps []string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if distName(d) == airflowDist {
			continue
		}
		out = append(out, d)
	}
	return out
}

// distName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
// Mirrors internal/imagebuild.distName.
func distName(req string) string {
	s := strings.TrimSpace(req)
	if i := strings.IndexAny(s, "[ \t<>=!~;@("); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

// packagesWarning returns a warning when the manifest declares OS packages,
// which neither MWAA nor Composer can install from a bucket artifact. The
// artifact is still valid — the dags and requirements are fine — so this is a
// warning, not a refusal, and it names the packages and the platform.
func packagesWarning(packages []string, platform string) string {
	if len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("%s installs no OS packages from this artifact; the manifest's packages (%s) are skipped. Ship them another way (a startup script, a plugins wheel, or a custom image where the platform allows one).",
		platform, strings.Join(packages, ", "))
}

// includeWarning flags an include/ folder with real files. MWAA and Composer
// have no include/ concept, so the target does not ship it, and a DAG that
// imports from include/ breaks at runtime — a warning is the honest signal to
// move shared modules under dags/.
func includeWarning(projectDir string) string {
	if !dirHasFiles(filepath.Join(projectDir, "include")) {
		return ""
	}
	return "include/ has files but is not shipped: MWAA and Composer have no include/ folder, so a DAG that imports from it will break. Move shared modules under dags/."
}

// envChecklist renders the manifest's declared environment values as a plain
// checklist, or "" when the manifest declares none. MWAA and Composer set these
// in their own consoles, so the artifact carries the list to configure, not the
// values. intro and note frame it for the platform.
func envChecklist(project string, env map[string]any, intro, note string) (string, error) {
	if len(env) == 0 {
		return "", nil
	}
	schema, err := envschema.ParseSchema(env)
	if err != nil {
		return "", err
	}
	if len(schema.EnvVars) == 0 && len(schema.AirflowVariables) == 0 && len(schema.Connections) == 0 {
		return "", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Environment setup for %s\n\n%s\n", project, intro)

	writeValues := func(title string, specs map[string]envschema.ValueSpec) {
		if len(specs) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		for _, name := range sortedKeys(specs) {
			fmt.Fprintf(&b, "- [ ] %s%s\n", name, valueMeta(specs[name]))
		}
	}
	writeValues("Environment variables", schema.EnvVars)
	writeValues("Airflow variables", schema.AirflowVariables)
	writeValues("Connections", schema.Connections)

	fmt.Fprintf(&b, "\n%s\n", note)
	return b.String(), nil
}

// valueMeta annotates a checklist entry with where its value comes from: a
// committed default, the workspace's Environment Manager, or neither. A value
// with neither is one the platform operator must supply, unless it is
// declared optional.
//
// The workspace case is tested first, even for an optional value, because
// where a value comes from is the more useful thing to say, and an optional
// workspace value still has to be set there to take effect. A default outranks
// optional for a similar reason: a value with one is satisfied either way.
// Only a value with neither falls to optional, so a Dag's nice-to-have
// setting does not read as something the environment cannot run without.
func valueMeta(spec envschema.ValueSpec) string {
	switch {
	case spec.Source == envschema.SourceWorkspace:
		return " (from workspace)"
	case spec.HasDefault:
		return " (has a default)"
	case spec.Optional:
		return " (optional)"
	default:
		return " (required)"
	}
}

func sortedKeys(m map[string]envschema.ValueSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// emit sends one progress line through the callbacks under the "package"
// component, the same channel the astro target's save streams through.
func emit(cb localrt.Callbacks, text string) {
	if cb.OnLine != nil {
		cb.OnLine(localrt.LogLine{Component: "package", Time: time.Now(), Text: text})
	}
}

// saveTree zips the artifact into req.Save when set and records it on the
// result. Both tree targets share the step: the tree is the artifact, and
// --save adds a single uploadable zip beside it.
func saveTree(req Request, outDir string, res *Result, cb localrt.Callbacks) error {
	if req.Save == "" {
		return nil
	}
	emit(cb, "saving "+req.Save)
	size, err := zipTree(outDir, req.Save)
	if err != nil {
		return fmt.Errorf("saving %s to %s: %w", outDir, req.Save, err)
	}
	res.SavedPath = req.Save
	res.Size = size
	return nil
}
