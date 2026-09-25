//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// `astro package` builds the artifact a managed platform wants from the
// project's files. It needs no Python environment at all — measured with uv off
// PATH and no .venv — so these are tier 0 rather than the tier 1 the plan
// assumed, and the sharpest assertions here run on every PR for free.

// tree lists everything under dir, project-relative and sorted, so a case can
// state the whole artifact instead of probing it a path at a time.
//
// Directories are listed too, with a trailing slash. Skipping them would have
// let an empty one through unnoticed, which is exactly what the cache-only
// plugins case is about: "produces no plugins.zip" and "writes no plugins
// directory at all" are different claims, and only the second is the one worth
// making.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := filepath.ToSlash(rel)
		if d.IsDir() {
			name += "/"
		}
		out = append(out, name)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

func TestPackageMWAAWritesTheBucketLayout(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()

	r := p.run("package", "mwaa").requireSuccess()

	got := tree(t, filepath.Join(p.Dir, "dist", "mwaa"))
	want := []string{"dags/", "dags/exampledag.py", "requirements.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("artifact tree = %v, want %v", got, want)
	}
	// The upload command is the whole point of the output: without it a user
	// has an artifact and no idea what to do with it.
	if !strings.Contains(r.Stdout, "aws s3 sync") {
		t.Errorf("expected the S3 upload command\n%s", r.output())
	}
	// The manifest pins an Airflow MWAA does not offer, so the run says so and
	// names what it does offer, rather than silently building for a version
	// the platform cannot run.
	//
	// The precondition is data: pkg/platformversions.MWAA is a hand-maintained
	// list, and the pin is runtimeversions.FallbackAirflowSeries, since the
	// harness keeps init offline. Should MWAA come to list it, there is no
	// mismatch to warn about and this case needs a different pin — so say so
	// here rather than in a bug report.
	if !strings.Contains(r.Stdout, "which MWAA does not list") {
		t.Errorf("expected a version-mismatch warning. If MWAA now lists the "+
			"scaffold's pinned Airflow, this case needs a pin that it does "+
			"not, rather than a fix\n%s", r.output())
	}
}

func TestPackageComposerWritesItsOwnLayout(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()

	r := p.run("package", "composer").requireSuccess()

	got := tree(t, filepath.Join(p.Dir, "dist", "composer"))
	want := []string{"composer-requirements.txt", "dags/", "dags/exampledag.py"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("artifact tree = %v, want %v", got, want)
	}
	if !strings.Contains(r.Stdout, "gcloud composer") {
		t.Errorf("expected the gcloud upload command\n%s", r.output())
	}
}

// Nothing from Python's bytecode cache reaches a customer's bucket.
//
// This is the assertion worth the most here. A __pycache__ carries the names of
// DAGs that have since been deleted, and .pyc files compiled by one interpreter
// are noise-to-garbage under another — so shipping the directory leaks a
// project's history and mixes interpreter versions in someone else's
// environment.
func TestPackageShipsNoBytecodeCache(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()

	// A cache holding a .pyc for a DAG that no longer exists, which is exactly
	// the leak: the file name is the deleted DAG's.
	cache := filepath.Join(p.Dir, "dags", "__pycache__")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(cache, "deleted_dag.cpython-312.pyc"), "bytecode")
	write(t, filepath.Join(cache, "exampledag.cpython-312.pyc"), "bytecode")

	p.run("package", "mwaa").requireSuccess()

	for _, f := range tree(t, filepath.Join(p.Dir, "dist", "mwaa")) {
		if strings.Contains(f, "__pycache__") || strings.HasSuffix(f, ".pyc") {
			t.Errorf("the artifact carries %s", f)
		}
		if strings.Contains(f, "deleted_dag") {
			t.Errorf("the artifact leaks a deleted DAG's name: %s", f)
		}
	}
}

// The rule is the cache directory, not the extension: a sourceless .pyc sitting
// beside a DAG is importable, so it is somebody's deliberate deployment choice
// and it ships.
func TestPackageShipsALooseCompiledFile(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()
	write(t, filepath.Join(p.Dir, "dags", "compiled_only.pyc"), "bytecode")

	p.run("package", "mwaa").requireSuccess()

	var found bool
	for _, f := range tree(t, filepath.Join(p.Dir, "dist", "mwaa")) {
		if f == "dags/compiled_only.pyc" {
			found = true
		}
	}
	if !found {
		t.Error("a loose .pyc beside a DAG is importable and should ship")
	}
}

// The corner worth poking: a plugins directory holding nothing but a bytecode
// cache must produce no plugins.zip at all.
//
// Not an empty one, and no instruction to deploy it — an MWAA environment
// pointed at a 22-byte zip is a broken deployment, and being told to point it
// there is worse than being told nothing.
func TestPackageMakesNoPluginsZipForACacheOnlyPluginsDir(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()

	cache := filepath.Join(p.Dir, "plugins", "__pycache__")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(cache, "helper.cpython-312.pyc"), "bytecode")

	r := p.run("package", "mwaa").requireSuccess()

	for _, f := range tree(t, filepath.Join(p.Dir, "dist", "mwaa")) {
		if strings.Contains(f, "plugins") {
			t.Errorf("expected no plugins artifact, got %s", f)
		}
	}
	if strings.Contains(r.Stdout, "plugins.zip") {
		t.Errorf("the run mentions a plugins.zip it did not write\n%s", r.output())
	}
}

// And the other side of that rule, so it is a rule and not an accident: real
// plugin source does produce the zip.
func TestPackageZipsRealPlugins(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pkg").requireSuccess()
	write(t, filepath.Join(p.Dir, "plugins", "my_plugin.py"), "# a real plugin\n")

	p.run("package", "mwaa").requireSuccess()

	var found bool
	for _, f := range tree(t, filepath.Join(p.Dir, "dist", "mwaa")) {
		if f == "plugins.zip" {
			found = true
		}
	}
	if !found {
		t.Error("a plugins directory with source in it should produce plugins.zip")
	}
}
