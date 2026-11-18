//go:build e2e && !windows

package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ptySpawn runs the CLI with a pseudo-terminal on stdin, stdout and stderr, as
// a person's shell gives it, using Python's pty module (the suite takes no
// dependency for it), and feeds stdin to the terminal as typed input. The
// shell command line is run by sh, so a redirect (2>file) is the shell's.
const ptySpawn = `import os, pty, sys
pid, fd = pty.fork()
if pid == 0:
    os.execvp("sh", ["sh", "-c", sys.argv[1]])
os.write(fd, sys.stdin.buffer.read())
while True:
    try:
        chunk = os.read(fd, 4096)
    except OSError:
        break
    if not chunk:
        break
    sys.stdout.buffer.write(chunk)
    sys.stdout.buffer.flush()
_, status = os.waitpid(pid, 0)
sys.exit(os.waitstatus_to_exitcode(status))
`

// runAtTerminal runs "astro args..." (shell-quoted by the caller where needed)
// at a pty, with typed as the keystrokes, and returns what the terminal showed.
func (p *project) runAtTerminal(typed string, extra map[string]string, shellLine string) *result {
	p.t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		p.t.Skip("a pty case needs python3")
	}
	cmd := exec.Command(py, "-I", "-c", ptySpawn, astroBin+" "+shellLine)
	cmd.Dir = p.Dir
	cmd.Env = p.env(extra)
	cmd.Stdin = strings.NewReader(typed)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	r := &result{t: p.t, Args: []string{shellLine}}
	err = cmd.Run()
	r.Stdout = strings.ReplaceAll(out.String(), "\r\n", "\n")
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		r.ExitCode = exit.ExitCode()
	default:
		p.t.Fatalf("running at a pty: %v", err)
	}
	return r
}

// A question is asked only of a person at a terminal, and the conditions that
// refuse it are each decided by the real binary: CI, Jenkins, Azure Pipelines
// and --non-interactive refuse at a terminal too, --non-interactive=false asks
// again despite CI, and a stderr that is not a terminal refuses even with
// stdin at one. The harness has no terminal (see TestNoTerminalNeverPrompts),
// so these run under a pty.
//
// Tier 0, `astro local reset` as in TestNoTerminalNeverPrompts.
func TestInteractionModeAtATerminal(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	write(t, filepath.Join(p.Dir, "pyproject.toml"), "[project]\nname = 'demo'\n[tool.astro]\n")

	asked := func(name string, r *result) {
		t.Helper()
		if !strings.Contains(r.Stdout, "[y/N]") || !strings.Contains(r.Stdout, "Error: aborted") || r.ExitCode != 1 {
			t.Errorf("%s: want the question asked and declined (aborted, exit 1)\n%s", name, r.output())
		}
	}
	refused := func(name string, r *result, why string) {
		t.Helper()
		if strings.Contains(r.Stdout, "[y/N]") {
			t.Errorf("%s: a question was written\n%s", name, r.output())
		}
		if r.ExitCode != 1 || !strings.Contains(r.Stdout, why) || !strings.Contains(r.Stdout, "pass --yes") {
			t.Errorf("%s: want input_required (%s) naming --yes, exit 1\n%s", name, why, r.output())
		}
	}

	asked("a terminal", p.runAtTerminal("n\n", nil, "local reset"))
	refused("CI=1", p.runAtTerminal("n\n", map[string]string{"CI": "1"}, "local reset"), "with CI set it cannot")
	asked("CI=1 --non-interactive=false", p.runAtTerminal("n\n", map[string]string{"CI": "1"}, "local reset --non-interactive=false"))
	refused("JENKINS_URL", p.runAtTerminal("n\n", map[string]string{"JENKINS_URL": "https://ci.example.com/"}, "local reset"), "in Jenkins")
	refused("TF_BUILD=True", p.runAtTerminal("n\n", map[string]string{"TF_BUILD": "True"}, "local reset"), "in Azure Pipelines")
	asked("TF_BUILD=False", p.runAtTerminal("n\n", map[string]string{"TF_BUILD": "False"}, "local reset"))
	refused("--non-interactive", p.runAtTerminal("n\n", nil, "local reset --non-interactive"), "with --non-interactive it cannot")

	// Ctrl-D with no answer is input_required, not a no.
	r := p.runAtTerminal("\x04", nil, "local reset")
	if !strings.Contains(r.Stdout, "[y/N]") || !strings.Contains(r.Stdout, "input ended with no answer") || r.ExitCode != 1 {
		t.Errorf("Ctrl-D: want the question and input_required (input ended with no answer)\n%s", r.output())
	}

	// stdin at a terminal, stderr a file: refused, and the reason is in the file.
	errFile := filepath.Join(t.TempDir(), "stderr.txt")
	r = p.runAtTerminal("n\n", nil, "local reset 2>"+errFile)
	b, err := os.ReadFile(errFile)
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 1 || !strings.Contains(string(b), "stderr is not a terminal") || strings.Contains(string(b), "[y/N]") {
		t.Errorf("stderr to a file: want a refusal naming stderr, no question\n%s\nstderr file: %s", r.output(), b)
	}
}
