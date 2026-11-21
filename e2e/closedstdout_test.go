//go:build e2e && !windows

package e2e

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// A read or listing command whose stdout reader has gone (`astro config list |
// head -1`) ends quietly with 0: the reader has what it wanted. It used to die
// of SIGPIPE (141) or print a write error.
//
// The reader here is gone before the command writes, which is what a reader
// that took its line and left looks like to a command with more to say, minus
// the race over how much fits in the pipe's buffer: a command whose whole
// output fits is not the case this is about, and no listing here is bigger
// than the buffer.
//
// Unix only: the Windows pipe error this relies on mapping (ERROR_NO_DATA) is
// covered by cliout's unit test, which cannot say how a real Windows pipe
// closes.
func TestAClosedStdoutEndsAListingWithZeroAndNoError(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	for _, args := range [][]string{
		{"config", "list"},
		{"config", "list", "-o", "json"},
		{"help"},
	} {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			code, stderr := p.forT(t).runWithClosedStdout(args...)
			if code != 0 || stderr != "" {
				t.Fatalf("`astro %s` with no reader on stdout: exit %d, stderr %q; want exit 0 and no stderr", name, code, stderr)
			}
		})
	}
}

// A closed stdout hides nothing else: a command that fails reports its error
// on stderr and exits with its own code, whether or not anyone reads stdout.
func TestAClosedStdoutDoesNotHideAFailure(t *testing.T) {
	tier(t, 0)

	code, stderr := newProject(t).runWithClosedStdout("config", "get", "no.such.key")
	if code != 2 || !strings.Contains(stderr, `unknown setting: "no.such.key"`) {
		t.Fatalf("exit %d, stderr %q; want the command's own exit 2 and its error", code, stderr)
	}
}

// runWithClosedStdout runs the CLI with its stdout a pipe nobody reads, and
// returns the exit code and what it wrote to stderr.
func (p *project) runWithClosedStdout(args ...string) (code int, stderr string) {
	p.t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		p.t.Fatal(err)
	}
	// The read end closes before the command starts, so its first write finds
	// no reader.
	r.Close()
	defer w.Close()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	var errOut strings.Builder
	cmd := exec.CommandContext(ctx, astroBin, args...)
	cmd.Dir = p.Dir
	cmd.Env = p.env(nil)
	cmd.Stdout = w
	cmd.Stderr = &errOut
	runErr := cmd.Run()
	if runErr == nil {
		return 0, errOut.String()
	}
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) {
		p.t.Fatalf("running `astro %s`: %v", strings.Join(args, " "), runErr)
	}
	if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), errOut.String()
	}
	return exit.ExitCode(), errOut.String()
}
