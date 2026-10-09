//go:build windows

package proxy

import (
	"net"
	"os"
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
