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

	"github.com/astronomer/astro-cli/internal/envresolve"
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

// copyDir copies the tree at src to dst, recreating directories and files. It
// skips nothing: the whole dags/ folder ships as-is.
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
			return os.MkdirAll(target, treeDirPerm)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks and other irregular entries
		}
		return copyFile(path, target)
	})
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
func dirHasFiles(dir string) bool {
	found := false
	//nolint:errcheck // a walk error just leaves found false
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
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
		if d.IsDir() || !d.Type().IsRegular() {
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
	schema, err := envresolve.ParseSchema(env)
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
// committed default, the workspace's Environment Manager, or neither (a value
// the platform operator must supply).
func valueMeta(spec envschema.ValueSpec) string {
	switch {
	case spec.Source == envschema.SourceWorkspace:
		return " (from workspace)"
	case spec.HasDefault:
		return " (has a default)"
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
