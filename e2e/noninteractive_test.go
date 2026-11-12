//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// A run with CI set never prompts, whatever stdin is: the question it would
// have asked fails as input_required, naming the flag that answers it, and
// nothing is written to stderr before the error, so a log shows no question
// that nobody could answer. --non-interactive does the same without CI, and
// --non-interactive=false asks despite CI. Every other run asks, on stderr,
// even with no terminal on stdin, and a stdin that ends with no answer is
// input_required too.
//
// Tier 0, as TestAJSONRunNeverPrompts: `astro local reset` finds the project
// first and its question is the first thing it would do.
func TestCINeverPrompts(t *testing.T) {
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
	asked := func(name string, r *result) {
		t.Helper()
		if r.ExitCode != 1 {
			t.Fatalf("%s: exit %d, want 1\n%s", name, r.ExitCode, r.output())
		}
		if !strings.Contains(r.Stderr, "[y/N]") {
			t.Errorf("%s: the question was not written on stderr\n%s", name, r.output())
		}
		if !strings.Contains(r.Stderr, "stdin is not interactive") || !strings.Contains(r.Stderr, "pass --yes") {
			t.Errorf("%s: want input_required naming --yes for the stdin that ended\n%s", name, r.output())
		}
	}

	refused("CI=true", p.runWith(map[string]string{"CI": "true"}, "local", "reset"), "with CI set it cannot")
	refused("CI=1", p.runWith(map[string]string{"CI": "1"}, "local", "reset"), "with CI set it cannot")
	refused("--non-interactive", p.run("local", "reset", "--non-interactive"), "with --non-interactive it cannot")
	refused("--non-interactive with CI=false", p.runWith(map[string]string{"CI": "false"}, "local", "reset", "--non-interactive"), "with --non-interactive it cannot")

	asked("CI unset", p.run("local", "reset"))
	asked("CI=false", p.runWith(map[string]string{"CI": "false"}, "local", "reset"))
	asked("CI=0", p.runWith(map[string]string{"CI": "0"}, "local", "reset"))
	asked("--non-interactive=false with CI=true", p.runWith(map[string]string{"CI": "true"}, "local", "reset", "--non-interactive=false"))

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
