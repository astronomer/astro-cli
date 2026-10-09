package proxy

import (
	"os"
	"testing"
)

// The stop sequence every platform shares: ask, then wait, then force. These
// run through the seams rather than real processes, so they hold on whichever
// OS runs them; the Windows hooks behind the seams are exercised in
// daemon_windows_test.go.

// stopSeams replaces the hooks the stop sequence goes through and records what
// was called.
type stopSeams struct {
	asked, waited, killed []int
}

func withStopSeams(t *testing.T, canAsk bool) *stopSeams {
	t.Helper()
	s := &stopSeams{}
	origAsk, origWait, origKill := requestStop, waitForDaemonExit, killProcess
	requestStop = func(pid int) bool { s.asked = append(s.asked, pid); return canAsk }
	waitForDaemonExit = func(pid int) { s.waited = append(s.waited, pid) }
	killProcess = func(pid int) { s.killed = append(s.killed, pid) }
	t.Cleanup(func() { requestStop, waitForDaemonExit, killProcess = origAsk, origWait, origKill })
	return s
}

// liveDaemonRecord writes a record naming this test process, which is alive on
// every platform, so the stop sequence believes a daemon is running.
func liveDaemonRecord(t *testing.T) *Daemon {
	t.Helper()
	d := &Daemon{Store: NewStore(t.TempDir())}
	if err := d.writeRecord(os.Getpid(), "6563"); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStopWaitsForADaemonItCouldAsk(t *testing.T) {
	s := withStopSeams(t, true)
	d := liveDaemonRecord(t)

	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if len(s.asked) != 1 || len(s.waited) != 1 || s.waited[0] != os.Getpid() {
		t.Errorf("asked %v, waited %v; want one ask and one wait for the recorded pid", s.asked, s.waited)
	}
	if _, err := os.Stat(d.RecordPath()); !os.IsNotExist(err) {
		t.Error("the record survived the stop")
	}
}

// A pid that cannot be asked is not ours to force: on Windows that is a live
// process with no stop event, most likely one that recycled a stale PID. Its
// record still goes, so the next start does not adopt it.
func TestStopLeavesAProcessItCouldNotAsk(t *testing.T) {
	s := withStopSeams(t, false)
	d := liveDaemonRecord(t)

	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if len(s.waited) != 0 || len(s.killed) != 0 {
		t.Errorf("waited %v, killed %v; want neither for a process that could not be asked", s.waited, s.killed)
	}
	if _, err := os.Stat(d.RecordPath()); !os.IsNotExist(err) {
		t.Error("the record survived the stop")
	}
}

func TestStopIfEmptyWaitsOnlyForADaemonItCouldAsk(t *testing.T) {
	for _, canAsk := range []bool{true, false} {
		s := withStopSeams(t, canAsk)
		d := liveDaemonRecord(t)

		d.StopIfEmpty()
		if len(s.asked) != 1 {
			t.Fatalf("canAsk=%v: asked %v, want one ask", canAsk, s.asked)
		}
		if got := len(s.waited) == 1; got != canAsk {
			t.Errorf("canAsk=%v: waited %v", canAsk, s.waited)
		}
	}
}

// waitForExit forces only a process that is still there once stopTimeout has
// passed.
func TestWaitForExitForcesOnlyAStragglingProcess(t *testing.T) {
	origAlive := IsPIDAlive
	t.Cleanup(func() { IsPIDAlive = origAlive })

	t.Run("exits in time", func(t *testing.T) {
		s := withStopSeams(t, true)
		IsPIDAlive = func(int) bool { return false }
		waitForExit(4242)
		if len(s.killed) != 0 {
			t.Errorf("killed %v, want nothing forced for a process that exited", s.killed)
		}
	})

	t.Run("straggles", func(t *testing.T) {
		if testing.Short() {
			t.Skip("waits out stopTimeout")
		}
		s := withStopSeams(t, true)
		IsPIDAlive = func(int) bool { return true }
		waitForExit(4242)
		if len(s.killed) != 1 || s.killed[0] != 4242 {
			t.Errorf("killed %v, want the straggler forced", s.killed)
		}
	})
}
