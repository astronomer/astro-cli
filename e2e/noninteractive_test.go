//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// A question needs a person at a terminal. The harness runs the binary with
// stdin from /dev/null and stderr a pipe, so every run here has no terminal: a
// prompt is refused before it is written or read, as input_required naming the
// flag that answers it, and an answer piped in is never read (`echo y | astro
// local reset` used to be how a script answered). CI, --non-interactive and
// json refuse the same way, and --non-interactive=false restores nothing
// without a terminal.
//
// Tier 0, as TestAJSONRunNeverPrompts: `astro local reset` finds the project
// first and its question is the first thing it would do.
func TestNoTerminalNeverPrompts(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	write(t, filepath.Join(p.Dir, "pyproject.toml"), "[project]\nname = 'demo'\n[tool.astro]\n")

	refused := func(name string, r *result, why string) {
		t.Helper()
		if r.ExitCode != 1 {
			t.Fatalf("%s: exit %d, want 1\n%s", name, r.ExitCode, r.output())
		}
		if r.Stdout != "" {
			t.Errorf("%s: stdout should be empty\n%s", name, r.output())
		}
		if strings.Contains(r.Stderr, "[y/N]") || strings.Count(r.Stderr, "\n") != 1 {
			t.Errorf("%s: a question was written\n%s", name, r.output())
		}
		if !strings.HasPrefix(r.Stderr, "Error: ") || !strings.Contains(r.Stderr, why) || !strings.Contains(r.Stderr, "pass --yes") {
			t.Errorf("%s: want input_required naming --yes (%s)\n%s", name, why, r.output())
		}
	}

	refused("stdin /dev/null", p.run("local", "reset"), "stdin is not a terminal")
	refused("a piped yes", p.runPiped("y\n", "local", "reset"), "stdin is not a terminal")
	refused("a piped no", p.runPiped("n\n", "local", "reset"), "stdin is not a terminal")
	refused("--non-interactive", p.run("local", "reset", "--non-interactive"), "with --non-interactive it cannot")
	refused("--non-interactive=false restores nothing without a terminal", p.run("local", "reset", "--non-interactive=false"), "stdin is not a terminal")
	refused("--non-interactive=false and a piped yes", p.runPiped("y\n", "local", "reset", "--non-interactive=false"), "stdin is not a terminal")
	refused("CI=true", p.runWith(map[string]string{"CI": "true"}, "local", "reset"), "stdin is not a terminal")
	refused("JENKINS_URL", p.runWith(map[string]string{"JENKINS_URL": "https://ci.example.com/"}, "local", "reset"), "stdin is not a terminal")
	refused("TF_BUILD=True", p.runWith(map[string]string{"TF_BUILD": "True"}, "local", "reset"), "stdin is not a terminal")

	// --yes is the answer a script gives, and is not refused.
	// (Not run here: it would wipe the project's state; the refusals above are
	// the point.)

	// json is never interactive, even with --non-interactive=false.
	r := p.runWith(map[string]string{"CI": "true"}, "local", "reset", "--non-interactive=false", "-o", "json")
	var emitted map[string]any
	r.requireJSON(&emitted)
	if emitted["kind"] != "input_required" {
		t.Errorf("want kind input_required, got %v", emitted)
	}
	if strings.Contains(r.Stderr, "[y/N]") {
		t.Errorf("a json run asked its question\n%s", r.output())
	}
}
