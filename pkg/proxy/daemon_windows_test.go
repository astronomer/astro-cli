//go:build windows

package proxy

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// freePort is a loopback port nothing is listening on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Serve listens on its stop event before it says it is serving, and returns
// once the event is set. The event is the only way to stop a daemon on
// Windows, so a Serve that wrote its port file first could be named by a record
// that nothing can stop.
func TestServeStopsWhenItsEventIsSet(t *testing.T) {
	d := &Daemon{Store: NewStore(t.TempDir())}
	port := freePort(t)
	served := make(chan error, 1)
	go func() { served <- d.Serve(port) }()

	waitFor(t, "the port file", func() bool {
		_, err := os.Stat(d.portPath())
		return err == nil
	})

	name, err := windows.UTF16PtrFromString(stopEventName(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		t.Fatalf("the stop event did not exist once the port file did: %v", err)
	}
	defer windows.CloseHandle(ev)
	if err := windows.SetEvent(ev); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after its stop event was set")
	}
}

// The stop event outlives the stop it delivers: it stays until release, which
// Serve defers, so a second stopper or an identity check during the graceful
// shutdown still finds it. Then it goes, so it never outlives the daemon.
func TestTheStopEventLastsUntilRelease(t *testing.T) {
	stop, release, err := stopRequests()
	if err != nil {
		t.Fatal(err)
	}
	if !requestStop(os.Getpid()) {
		t.Fatal("requestStop found no stop event for a process listening for one")
	}
	t.Cleanup(func() { forgetStop(os.Getpid()) })
	select {
	case <-stop:
	case <-time.After(10 * time.Second):
		t.Fatal("the stop channel did not close after the event was set")
	}

	if !processLooksLikeProxy(nil, os.Getpid()) {
		t.Error("the stop event was gone during shutdown, before release")
	}
	release()
	if processLooksLikeProxy(nil, os.Getpid()) {
		t.Error("the stop event outlived release")
	}
}

// The test binary doubles as the daemon and as a bystander process, picked by
// environment variable, so these tests spawn real detached processes without
// needing an astro build.
const (
	helperServeEnv = "ASTRO_PROXY_TEST_SERVE_DIR"
	helperIdleEnv  = "ASTRO_PROXY_TEST_IDLE"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperServeEnv); dir != "" {
		// Spawned by Daemon.Start as <exe> serve --port <port>. The port is
		// parsed as the number it is rather than passed through as a raw
		// argument, which gosec traces into the files Serve writes.
		port, err := strconv.Atoi(os.Args[len(os.Args)-1])
		if err != nil {
			os.Exit(1)
		}
		d := &Daemon{Store: NewStore(dir)}
		if err := d.Serve(strconv.Itoa(port)); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv(helperIdleEnv) != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A real daemon, start to stop: EnsureRunning spawns it detached and records
// it, a second EnsureRunning adopts it by its signature rather than starting
// another, and Stop ends it through its event.
func TestDaemonStartsAdoptsAndStopsOnWindows(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(helperServeEnv, dir)
	d := &Daemon{Store: NewStore(dir), Exe: os.Args[0], ServeArgs: []string{"serve"}, Version: "test"}

	port, err := d.EnsureRunning(freePort(t))
	if err != nil {
		t.Fatalf("EnsureRunning: %v (log: %s)", err, readLog(d))
	}
	pid, alive := d.IsRunning()
	if !alive {
		t.Fatal("no live daemon after EnsureRunning")
	}
	// Held for the test's life, so the cleanup can only ever end this daemon,
	// and only if the test failed before Stop did.
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ev, _ := windows.WaitForSingleObject(h, 0); ev == uint32(waitTimeout) {
			windows.TerminateProcess(h, 1) //nolint:errcheck // a failed test's leftover
		}
		windows.CloseHandle(h) //nolint:errcheck // nothing to do if it fails
	})
	if !probeProxySignature(port) {
		t.Fatalf("port %s does not answer as the proxy", port)
	}

	again, err := d.EnsureRunning(port)
	if err != nil {
		t.Fatal(err)
	}
	if again != port {
		t.Errorf("second EnsureRunning = %s, want the running daemon's %s", again, port)
	}
	if p2, _ := d.IsRunning(); p2 != pid {
		t.Errorf("second EnsureRunning replaced the daemon (pid %d, was %d)", p2, pid)
	}

	// A routes file the landing page cannot read makes the signature probe
	// fail. The daemon is still the daemon, known by its stop event, so it is
	// adopted rather than orphaned beside a second one.
	if err := os.WriteFile(d.Store.routesFilePath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if probeProxySignature(port) {
		t.Fatal("the probe still passes with an unreadable routes file; this case tests nothing")
	}
	if _, err := d.EnsureRunning(port); err != nil {
		t.Fatal(err)
	}
	if p3, _ := d.IsRunning(); p3 != pid {
		t.Errorf("a failed probe replaced the daemon (pid %d, was %d)", p3, pid)
	}
	if err := os.Remove(d.Store.routesFilePath()); err != nil {
		t.Fatal(err)
	}

	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the daemon to exit", func() bool { return !IsPIDAlive(pid) })
	if _, err := os.Stat(d.RecordPath()); !os.IsNotExist(err) {
		t.Error("the record survived the stop")
	}
}

// A live process with no stop event is not ours, so asking it to stop reports
// false and nothing ends it.
func TestRequestStopLeavesAProcessWithNoEvent(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperIdleEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	pid := cmd.Process.Pid

	if processLooksLikeProxy(nil, pid) {
		t.Error("a process with no stop event was taken for the daemon")
	}
	if requestStop(pid) {
		t.Error("requestStop reported a stop for a process with no stop event")
	}
	// No stop reached it, so nothing forces it either.
	killProcess(pid)
	time.Sleep(200 * time.Millisecond)
	if !IsPIDAlive(pid) {
		t.Error("the bystander died")
	}
}

func readLog(d *Daemon) string {
	b, _ := os.ReadFile(d.LogPath())
	return string(b)
}
