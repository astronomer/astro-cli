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
	defer windows.CloseHandle(ev) //nolint:errcheck // a test handle
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

// The test binary doubles as the daemon and as a bystander process, picked by
// environment variable, so these tests spawn real detached processes without
// needing an astro build.
const (
	helperServeEnv = "ASTRO_PROXY_TEST_SERVE_DIR"
	helperIdleEnv  = "ASTRO_PROXY_TEST_IDLE"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperServeEnv); dir != "" {
		// Spawned by Daemon.Start as <exe> serve --port <port>.
		d := &Daemon{Store: NewStore(dir)}
		if err := d.Serve(os.Args[len(os.Args)-1]); err != nil {
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
	t.Cleanup(func() { killProcess(pid) })
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
	t.Cleanup(func() { cmd.Process.Kill() }) //nolint:errcheck // test cleanup
	pid := cmd.Process.Pid

	if requestStop(pid) {
		t.Error("requestStop reported a stop for a process with no stop event")
	}
	time.Sleep(200 * time.Millisecond)
	if !IsPIDAlive(pid) {
		t.Error("the bystander died")
	}
}

func readLog(d *Daemon) string {
	b, _ := os.ReadFile(d.LogPath()) //nolint:errcheck // diagnostics only
	return string(b)
}
