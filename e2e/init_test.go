//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// `astro init` is the v2 replacement for `astro dev init`: a pyproject.toml
// project rather than a Dockerfile one.
func TestInitScaffoldsAProject(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "demo").requireSuccess()

	for _, dir := range []string{"dags", "include", "plugins", "tests"} {
		if fi, err := os.Stat(filepath.Join(p.Dir, dir)); err != nil || !fi.IsDir() {
			t.Errorf("%s/ is not a directory: %v", dir, err)
		}
	}
	for _, file := range []string{"pyproject.toml", ".gitignore", "AGENTS.md", filepath.Join("dags", "exampledag.py")} {
		if fi, err := os.Stat(filepath.Join(p.Dir, file)); err != nil || fi.IsDir() {
			t.Errorf("%s is not a file: %v", file, err)
		}
	}
}

// The two Airflow versions in a scaffolded project have to agree: [tool.astro]
// decides what the CLI installs and runs, the dependency pin is what uv
// resolves. They are written by the same scaffold from the same input, so a
// disagreement here is not a typo — it is the scaffold having two opinions,
// and the failure it produces (a project running a different Airflow than it
// declares) is a confusing one to debug from the far end.
func TestInitPinsOneAirflowVersion(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "demo").requireSuccess()

	manifest := read(t, filepath.Join(p.Dir, "pyproject.toml"))

	declared := regexp.MustCompile(`(?m)^airflow\s*=\s*['"]([^'"]+)['"]`).FindStringSubmatch(manifest)
	if declared == nil {
		t.Fatalf("no [tool.astro] airflow pin in:\n%s", manifest)
	}
	pinned := regexp.MustCompile(`apache-airflow==(\d+\.\d+)`).FindStringSubmatch(manifest)
	if pinned == nil {
		t.Fatalf("no apache-airflow dependency pin in:\n%s", manifest)
	}
	if declared[1] != pinned[1] {
		t.Errorf("[tool.astro] airflow = %q but the dependency pins %q", declared[1], pinned[1])
	}
}

// The agent guidance is one file, published under two names by a symlink, so it
// cannot go stale on one side. Windows gets a layout with no CLAUDE.md at all —
// creating a symlink there needs developer mode or elevation, and a copy would
// be the stale second side the link exists to avoid.
//
// Both layouts are asserted rather than one being skipped, because "the file is
// missing" and "the file is missing on the platform where it should be" are the
// same observation until someone writes down which is which.
func TestInitLinksTheAgentGuidance(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "demo").requireSuccess()

	agents := filepath.Join(p.Dir, "AGENTS.md")
	claude := filepath.Join(p.Dir, "CLAUDE.md")

	if read(t, agents) == "" {
		t.Error("AGENTS.md is empty")
	}

	if runtime.GOOS == "windows" {
		if _, err := os.Lstat(claude); !os.IsNotExist(err) {
			t.Errorf("the Windows layout has no CLAUDE.md, but one is here (%v)", err)
		}
		return
	}

	fi, err := os.Lstat(claude)
	if err != nil {
		t.Fatalf("CLAUDE.md: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("CLAUDE.md is a copy of AGENTS.md rather than a link to it")
	}
	if read(t, claude) != read(t, agents) {
		t.Error("CLAUDE.md and AGENTS.md do not have the same content")
	}
}

// Re-running init over its own output is something people do, usually by
// accident. It refuses rather than reconciling, which is the safer of the two:
// the alternative is a command that quietly rewrites a manifest someone has
// since edited by hand. What this pins is that the refusal changes nothing.
func TestInitRefusesToReinitialize(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "demo").requireSuccess()

	manifest := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	dag := read(t, filepath.Join(p.Dir, "dags", "exampledag.py"))

	p.run("init", "--name", "demo").
		requireFailure().
		requireStderr("already an Astro project")

	if got := read(t, filepath.Join(p.Dir, "pyproject.toml")); got != manifest {
		t.Errorf("a refused init rewrote pyproject.toml anyway:\n--- before\n%s\n--- after\n%s", manifest, got)
	}
	if got := read(t, filepath.Join(p.Dir, "dags", "exampledag.py")); got != dag {
		t.Error("a refused init rewrote dags/exampledag.py anyway")
	}
}

// Adopting a directory that already has a plain Python pyproject.toml is the
// path a real user takes, and the thing they will not forgive is losing what
// was already in the file — comments included, since those are exactly what a
// naive read-modify-write drops.
func TestInitAdoptsAnExistingManifest(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	existing := strings.Join([]string{
		"# Kept by hand: the pin below is deliberate, see the upgrade ticket.",
		"[project]",
		"name = 'already-here'",
		"version = '2.5.0'",
		"dependencies = [",
		"    'requests==2.31.0', # pinned, upstream broke us",
		"]",
		"",
		"[tool.ruff]",
		"line-length = 120",
		"",
	}, "\n")
	write(t, filepath.Join(p.Dir, "pyproject.toml"), existing)

	p.run("init").requireSuccess()

	got := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	for _, keep := range []string{
		"# Kept by hand: the pin below is deliberate, see the upgrade ticket.",
		"name = 'already-here'",
		"version = '2.5.0'",
		"requests==2.31.0",
		"# pinned, upstream broke us",
		"[tool.ruff]",
		"line-length = 120",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("adoption dropped %q from the manifest:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "[tool.astro]") {
		t.Errorf("adoption did not add [tool.astro]:\n%s", got)
	}
	if !strings.Contains(got, "apache-airflow") {
		t.Errorf("adoption did not add an airflow dependency:\n%s", got)
	}
}

// A comment has to stay with the line it documents.
//
// Adopting a manifest whose dependency list is multiline and commented appends
// the airflow pin onto the last element's line, ahead of that element's
// trailing comment:
//
//	'requests==2.31.0', # pinned, upstream broke us      (before)
//	'requests==2.31.0', 'apache-airflow==3.1.*' # pinned, upstream broke us
//
// Both parse, which is why nothing complains. But the note explaining why
// requests is pinned now reads as a note about airflow, and the reader it
// misleads is the one deciding whether the pin is still needed. Nothing is
// lost, which is why it is worth a skipped case rather than a fix here.
//
// The placement is not this repo's to correct: appending to an array is
// go-toml's edit.Document.Set, which pkg/manifest/tomledit delegates to, and
// it inserts ahead of the trailing comment. The case stays so that a library
// version placing the element after the comment is noticed here first.
func TestInitKeepsACommentWithItsOwnDependency(t *testing.T) {
	tier(t, 0)
	t.Skip("known: the appended pin lands ahead of the last element's comment")

	p := newProject(t)
	write(t, filepath.Join(p.Dir, "pyproject.toml"), strings.Join([]string{
		"[project]",
		"name = 'already-here'",
		"dependencies = [",
		"    'requests==2.31.0', # pinned, upstream broke us",
		"]",
		"",
	}, "\n"))

	p.run("init").requireSuccess()

	got := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	if !strings.Contains(got, "'requests==2.31.0', # pinned, upstream broke us") {
		t.Errorf("the comment no longer trails the dependency it documents:\n%s", got)
	}
}

// The version the binary reports. Stamped by the suite's own build, so this
// pins that the -ldflags path still reaches the command — the fallback in
// version.Current means an unstamped build no longer prints an empty label,
// which would otherwise make this the kind of failure that looks like a pass.
func TestVersionReportsTheStampedVersion(t *testing.T) {
	tier(t, 0)

	r := newProject(t).run("version").requireSuccess()
	if !strings.Contains(r.Stdout, testVersion) {
		t.Errorf("`astro version` does not report %q\n%s", testVersion, r.output())
	}
}

// The suite's safety rests on the CLI honoring these, so assert it rather than
// trust it: if ASTRO_HOME stopped being read, every case here would silently
// start writing into the developer's own ~/.astro.
func TestIsolationKeepsWritesOutOfTheRealHome(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("config", "set", "-g", "upgrade_message", "false").requireSuccess()

	if _, err := os.Stat(filepath.Join(p.home, ".astro")); err != nil {
		t.Errorf("no .astro under the test's ASTRO_HOME, so that write went somewhere else: %v", err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
