//go:build !windows

package proxy

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// A long-lived starter that stops the daemon it started gets it back at once,
// with nothing left over. The starter is the daemon's parent until it exits,
// and a child it never waits for stays a zombie that signal 0 still reports
// alive: the stop would sit out the whole stopTimeout, then SIGKILL the corpse,
// and leave a <defunct> process behind. This test process is that starter, the
// way any long-running host is.
func TestStoppingTheDaemonItStartedLeavesNoZombie(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(helperServeEnv, dir)
	d := &Daemon{Store: NewStore(dir), Exe: os.Args[0], ServeArgs: []string{"serve"}, Version: "test"}

	if _, err := d.EnsureRunning(freePort(t)); err != nil {
		t.Fatalf("EnsureRunning: %v (log: %s)", err, readLog(d))
	}
	pid, alive := d.IsRunning()
	if !alive {
		t.Fatal("no live daemon after EnsureRunning")
	}
	// Only matters if Stop failed.
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	start := time.Now()
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Stop took %s, want well under stopTimeout (%s): the exited daemon was left unreaped", took.Round(time.Millisecond), stopTimeout)
	}
	if IsPIDAlive(pid) {
		t.Errorf("pid %d still reads as alive after Stop: a zombie the starter never waited for", pid)
	}
}
