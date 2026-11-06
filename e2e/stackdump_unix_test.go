//go:build e2e && !windows

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stackDumpGrace is how long a command given SIGQUIT has to print its stacks
// and exit before it is killed. The runtime writes them and exits at once, so
// this only matters for a command that does not: one that traps the signal,
// or leaves a child holding its output pipes open.
const stackDumpGrace = 10 * time.Second

// dumpStacksOnTimeout makes cmd, when its context's deadline passes, end with
// SIGQUIT rather than the SIGKILL exec.CommandContext sends by default. A Go
// program prints every goroutine's stack to stderr on SIGQUIT and exits 2, so
// the harness's "did not finish within" failure shows where the CLI was
// stuck. Without it a hang that does not reproduce locally says nothing
// about its cause.
//
// WaitDelay kills the process if it is still running stackDumpGrace later,
// so the bound still holds, a little later than before.
func dumpStacksOnTimeout(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGQUIT) }
	cmd.WaitDelay = stackDumpGrace
}

// hangingProgram blocks in a goroutine of its own and sleeps in main, so its
// stack dump has to name both for the dump to be every goroutine's. main
// sleeps rather than blocking on a channel: with every goroutine blocked on
// one, the runtime reports a deadlock and exits by itself.
const hangingProgram = `package main

import "time"

func blockedInHelper(c chan struct{}) { <-c }

func main() {
	c := make(chan struct{})
	go blockedInHelper(c)
	time.Sleep(time.Hour)
}
`

// A command that outlives its bound ends with every goroutine's stack on its
// stderr, not a bare kill: the point is a hang CI hits and nobody can
// reproduce, whose failure must say where the CLI was stuck.
func TestATimedOutCommandDumpsItsStacks(t *testing.T) {
	tier(t, 0)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hang\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(hangingProgram), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "hang")
	build := exec.Command("go", "build", "-race=false", "-o", bin, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the hanging program: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stderr = &stderr
	dumpStacksOnTimeout(cmd)

	start := time.Now()
	err := cmd.Run()
	if ctx.Err() == nil {
		t.Fatalf("the hanging program exited before its bound: %v", err)
	}
	if took := time.Since(start); took > stackDumpGrace {
		t.Errorf("it took %s to end; SIGQUIT should end a Go program at once, well inside the %s grace", took, stackDumpGrace)
	}
	out := stderr.String()
	for _, want := range []string{"SIGQUIT", "main.main", "main.blockedInHelper"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr lacks %q, so the dump is not every goroutine's stack:\n%s", want, out)
		}
	}
}
