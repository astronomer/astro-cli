//go:build !windows

package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDaemon is a daemon over a temp store, with an executable that does
// not exist: every test that reaches a start replaces startDaemon, and one that
// does not fails loudly rather than spawning anything.
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	return &Daemon{
		Store:     NewStore(t.TempDir()),
		Exe:       "/nonexistent/astro",
		ServeArgs: []string{"__proxy-serve"},
	}
}

// writeRawRecord writes the record file byte for byte, for the shapes older
// builds left behind.
func writeRawRecord(t *testing.T, d *Daemon, raw string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, os.WriteFile(d.RecordPath(), []byte(raw), 0o644))
}

func swapStart(t *testing.T, fn func(*Daemon, string) (string, error)) {
	t.Helper()
	orig := startDaemon
	startDaemon = fn
	t.Cleanup(func() { startDaemon = orig })
}

func swapIsProxy(t *testing.T, fn func(*Daemon, int, string) bool) {
	t.Helper()
	orig := isProxyDaemon
	isProxyDaemon = fn
	t.Cleanup(func() { isProxyDaemon = orig })
}

func swapAlive(t *testing.T, fn func(int) bool) {
	t.Helper()
	orig := IsPIDAlive
	IsPIDAlive = fn
	t.Cleanup(func() { IsPIDAlive = orig })
}

// recordingStart stands in for a real start: it records a live daemon (this
// test process) the way the real one does, and counts how often it ran.
func recordingStart(t *testing.T, starts *atomic.Int32) {
	t.Helper()
	swapStart(t, func(d *Daemon, port string) (string, error) {
		starts.Add(1)
		if err := os.MkdirAll(d.Store.Dir(), 0o755); err != nil {
			return "", err
		}
		return port, d.writeRecord(os.Getpid(), port)
	})
}

func TestIsRunning_NoRecord(t *testing.T) {
	d := newTestDaemon(t)
	_, alive := d.IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_StaleRecord(t *testing.T) {
	d := newTestDaemon(t)
	writeRawRecord(t, d, "99999999")
	swapAlive(t, func(int) bool { return false })

	_, alive := d.IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_AlivePID(t *testing.T) {
	d := newTestDaemon(t)
	pid := os.Getpid()
	writeRawRecord(t, d, strconv.Itoa(pid)+" 1.0.0")

	gotPid, alive := d.IsRunning()
	assert.True(t, alive)
	assert.Equal(t, pid, gotPid)
}

func TestIsRunning_AlivePID_NoVersion(t *testing.T) {
	d := newTestDaemon(t)
	pid := os.Getpid()
	writeRawRecord(t, d, strconv.Itoa(pid))

	gotPid, alive := d.IsRunning()
	assert.True(t, alive)
	assert.Equal(t, pid, gotPid)
}

func TestRecord_WithPort(t *testing.T) {
	d := newTestDaemon(t)
	pid := os.Getpid()
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, d.writeRecord(pid, "16123"))

	rec, err := d.readRecord()
	require.NoError(t, err)
	assert.Equal(t, pid, rec.PID)
	// No version set; writeRecord stores "-", which reads back empty.
	assert.Equal(t, "", rec.Version)
	assert.Equal(t, "16123", rec.Port)

	assert.Equal(t, "16123", d.BoundPort())
}

func TestBoundPort_OldRecordWithoutPort(t *testing.T) {
	d := newTestDaemon(t)
	writeRawRecord(t, d, strconv.Itoa(os.Getpid())+" 1.0.0")

	assert.Equal(t, "", d.BoundPort())
}

func TestEnsureRunning_AlreadyRunningReturnsBoundPort(t *testing.T) {
	d := newTestDaemon(t)
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, d.writeRecord(os.Getpid(), "16123"))

	// Simulate a genuine running proxy: the identity check confirms it.
	swapIsProxy(t, func(*Daemon, int, string) bool { return true })

	// Daemon alive with recorded port — EnsureRunning must report that port,
	// not the requested one.
	port, err := d.EnsureRunning("6563")
	require.NoError(t, err)
	assert.Equal(t, "16123", port)
}

// A live process that answers HTTP but isn't the proxy (no signature header)
// stands in for an unrelated process that recycled the daemon's old PID/port.
// EnsureRunning must treat the record as stale and start a fresh daemon.
func TestEnsureRunning_LiveNonProxyIsStale(t *testing.T) {
	d := newTestDaemon(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")

	// Record: our own PID (definitely alive) pointed at the non-proxy port.
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, d.writeRecord(os.Getpid(), port))

	// Pin the process-name fallback: the recycled process is not the proxy.
	origProc := processLooksLikeProxy
	processLooksLikeProxy = func(*Daemon, int) bool { return false }
	t.Cleanup(func() { processLooksLikeProxy = origProc })

	var starts atomic.Int32
	recordingStart(t, &starts)

	got, err := d.EnsureRunning("6563")
	require.NoError(t, err)
	assert.Equal(t, int32(1), starts.Load(), "a live non-proxy process on the recorded port must trigger a restart")
	assert.Equal(t, "6563", got)
}

// A live process that answers with the proxy signature is trusted as running,
// so EnsureRunning returns its port without restarting.
func TestEnsureRunning_LiveProxyIsTrusted(t *testing.T) {
	d := newTestDaemon(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(SignatureHeader, SignatureValue)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")

	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, d.writeRecord(os.Getpid(), port))

	swapStart(t, func(*Daemon, string) (string, error) {
		t.Fatal("Start must not run when a live proxy answers on the recorded port")
		return "", nil
	})

	got, err := d.EnsureRunning("6563")
	require.NoError(t, err)
	assert.Equal(t, port, got)
}

func TestEnsureRunning_ConcurrentStartsOnlyOne(t *testing.T) {
	d := newTestDaemon(t)

	// Once the first goroutine records a daemon, the rest must see a live
	// one: treat the recorded process as a genuine proxy.
	swapIsProxy(t, func(*Daemon, int, string) bool { return true })
	var starts atomic.Int32
	recordingStart(t, &starts)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	ports := make([]string, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A Daemon per goroutine, over the one store, the way separate
			// processes would each build their own.
			own := &Daemon{Store: d.Store, Exe: d.Exe, ServeArgs: d.ServeArgs}
			ports[i], errs[i] = own.EnsureRunning("6563")
		}()
	}
	wg.Wait()

	for i := range 8 {
		require.NoError(t, errs[i])
		assert.Equal(t, "6563", ports[i])
	}
	// The routes lock serializes check-then-start: exactly one goroutine may
	// find no daemon and start one.
	assert.Equal(t, int32(1), starts.Load())
}

// BeforeStart runs before a start, under the lock and with the port about to
// be bound, and not at all when a running daemon is adopted.
func TestEnsureRunning_RunsBeforeStartOnlyBeforeAStart(t *testing.T) {
	d := newTestDaemon(t)
	var calls []string
	var startedAfter atomic.Bool
	d.BeforeStart = func(port string) { calls = append(calls, port) }
	swapStart(t, func(d *Daemon, port string) (string, error) {
		startedAfter.Store(len(calls) == 1)
		if err := os.MkdirAll(d.Store.Dir(), 0o755); err != nil {
			return "", err
		}
		return port, d.writeRecord(os.Getpid(), port)
	})
	swapIsProxy(t, func(*Daemon, int, string) bool { return true })

	_, err := d.EnsureRunning("7001")
	require.NoError(t, err)
	assert.Equal(t, []string{"7001"}, calls)
	assert.True(t, startedAfter.Load(), "BeforeStart has to run before the start, not after")

	_, err = d.EnsureRunning("7001")
	require.NoError(t, err)
	assert.Len(t, calls, 1, "adopting a running daemon must not run BeforeStart")
}

func TestAddRoute_PersistsWhenDaemonFails(t *testing.T) {
	d := newTestDaemon(t)

	// The engines add a route before EnsureRunning. Even when the daemon
	// cannot start, routes.json must be populated so other tools can discover
	// the project.
	swapStart(t, func(*Daemon, string) (string, error) {
		return "", fmt.Errorf("proxy daemon is not supported here")
	})

	route := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        0,
		Services:   map[string]string{"postgres": "15432"},
		Mode:       "docker",
	}
	require.NoError(t, d.Store.AddRoute(route))

	_, err := d.EnsureRunning("6563")
	assert.Error(t, err)

	routes, err := d.Store.ListRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
	assert.Equal(t, "docker", routes[0].Mode)
	assert.Equal(t, "15432", routes[0].Services["postgres"])
}

func TestStartRefusesWithoutAnExecutable(t *testing.T) {
	d := newTestDaemon(t)
	d.Exe = ""
	_, err := d.Start("6563")
	assert.ErrorContains(t, err, "no executable")
}

func TestStopIfEmpty_NoRoutes(t *testing.T) {
	newTestDaemon(t).StopIfEmpty()
}

// The decision and the handover happen under the routes lock.
//
// In a gap between deciding and signaling, a concurrent start registers a
// route, finds the daemon alive and adopts it — EnsureRunning holds this same
// lock across its own check-then-start to close exactly that — and the SIGTERM
// lands on the daemon the start now depends on.
//
// Asserted by holding the lock and watching StopIfEmpty wait for it. The
// symptom needs two processes racing a real daemon to show itself, and a test
// that has to lose a race to pass is a test that passes once the race is gone.
func TestStopIfEmptyWaitsForTheRoutesLock(t *testing.T) {
	d := newTestDaemon(t)

	lockFile, err := d.Store.AcquireLock()
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		d.StopIfEmpty()
		close(done)
	}()
	// Joined before the test returns however it ends: StopIfEmpty reads files
	// under a TempDir the framework removes, so leaving it running is a data
	// race reported exactly when a real regression is being diagnosed.
	defer func() {
		ReleaseLock(lockFile)
		<-done
	}()

	select {
	case <-done:
		t.Fatal("StopIfEmpty ran to completion while another holder had the routes lock")
	case <-time.After(300 * time.Millisecond):
		// Still waiting, which is the point.
	}
}

// And the lock is NOT held while the daemon is being waited for, and by the
// time the wait starts the record is already gone, which is what stops a
// concurrent start from adopting a daemon that has been signaled.
func TestStopIfEmptyReleasesTheLockBeforeWaiting(t *testing.T) {
	d := newTestDaemon(t)

	// A pid nothing owns, reported alive, so the stop path runs all the way
	// to the wait while the SIGTERM it sends lands on nobody — ESRCH, ignored.
	// Signaling os.Getpid() here would terminate the test binary.
	const absentPID = 99999999
	swapAlive(t, func(int) bool { return true })

	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, WriteRecord(d.RecordPath(), Record{PID: absentPID, Version: "test", Port: "6563"}))

	orig := waitForDaemonExit
	t.Cleanup(func() { waitForDaemonExit = orig })

	var lockFree, recordGone atomic.Bool
	waitForDaemonExit = func(int) {
		if f, err := d.Store.AcquireLock(); err == nil {
			lockFree.Store(true)
			ReleaseLock(f)
		}
		if _, err := os.Stat(d.RecordPath()); os.IsNotExist(err) {
			recordGone.Store(true)
		}
	}

	d.StopIfEmpty()

	if !lockFree.Load() {
		t.Error("the routes lock was still held while waiting for the daemon")
	}
	if !recordGone.Load() {
		t.Error("the record still named the daemon being stopped, so a concurrent start could adopt it")
	}
}

func TestStopIfEmptyCountsOnlyLiveRoutes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		routeLive bool
		docker    bool
		wantStop  bool
	}{
		{name: "a live route keeps the daemon", routeLive: true, wantStop: false},
		{name: "a stale route does not", routeLive: false, wantStop: true},
		// Docker routes are never pruned by pid — the CLI exits after starting
		// the containers, so the recorded process says nothing about them — so
		// one left behind keeps the daemon up for good. That is the orphan the
		// CLI's `astro local list --clean` sweep exists to collect, and pinning
		// it here makes the leak visible if the predicate ever changes.
		{name: "a docker route is never stale to this", routeLive: false, docker: true, wantStop: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDaemon(t)
			swapAlive(t, func(int) bool { return tc.routeLive })

			route := &Route{
				Hostname:   "one.localhost",
				Port:       "8080",
				ProjectDir: t.TempDir(),
				PID:        os.Getpid(),
			}
			if tc.docker {
				route.Mode = RouteModeDocker
			}
			require.NoError(t, d.Store.AddRoute(route))

			// A record for a daemon that is not alive, so deciding to stop
			// removes it and deciding otherwise leaves it. No signal is sent
			// either way, so the decision is observable without a seam.
			require.NoError(t, WriteRecord(d.RecordPath(), Record{PID: 99999999, Version: "test", Port: "6563"}))

			d.StopIfEmpty()

			_, err := os.Stat(d.RecordPath())
			stopped := os.IsNotExist(err)
			if stopped != tc.wantStop {
				t.Errorf("decided to stop the daemon = %v, want %v", stopped, tc.wantStop)
			}
		})
	}
}

func TestStartProxyRemembersItsFallbackPort(t *testing.T) {
	d := newTestDaemon(t)
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	taken := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)

	first, err := d.startProxy(taken)
	require.NoError(t, err)
	fallback := first.Port()
	first.Stop()
	require.NotEqual(t, taken, fallback)

	second, err := d.startProxy(taken)
	require.NoError(t, err)
	defer second.Stop()
	assert.Equal(t, fallback, second.Port(), "the second start should reuse the port the first fell back to")
}

// Whether a running daemon is reused is the protocol's to decide, and nothing
// else's: not the version of the tool that started it.
//
// The record is written raw, the way each generation of daemon leaves it,
// rather than through writeRecord, which only ever writes this protocol.
func TestEnsureRunningReusesByProtocol(t *testing.T) {
	for _, tc := range []struct {
		name      string
		record    string
		wantReuse bool
	}{
		{"the same protocol is reused", fmt.Sprintf("%%d SNAPSHOT-abc 16123 %d", DaemonProtocol), true},
		{"a newer protocol is reused", fmt.Sprintf("%%d 2.0.0 16123 %d", DaemonProtocol+1), true},
		{"an older protocol is replaced", fmt.Sprintf("%%d 9.9.9 16123 %d", DaemonProtocol-1), false},
		// Every daemon from before protocols, whatever its version, including
		// one newer than the tool asking: the version no longer decides.
		{"a record from before protocols is replaced", "%d 9.9.9 16123", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDaemon(t)
			d.Version = "2.0.0"

			// A pid nothing owns, reported alive: replacing the daemon signals
			// it, and signaling this test's own pid would end the test.
			const absentPID = 99999999
			swapAlive(t, func(pid int) bool { return pid == absentPID })
			writeRawRecord(t, d, fmt.Sprintf(tc.record, absentPID))
			swapIsProxy(t, func(*Daemon, int, string) bool { return true })
			orig := waitForDaemonExit
			waitForDaemonExit = func(int) {}
			t.Cleanup(func() { waitForDaemonExit = orig })

			var starts atomic.Int32
			swapStart(t, func(d *Daemon, port string) (string, error) {
				starts.Add(1)
				return "17000", nil
			})

			port, err := d.EnsureRunning("6563")
			require.NoError(t, err)
			if tc.wantReuse {
				assert.Equal(t, int32(0), starts.Load(), "a reusable daemon was replaced")
				assert.Equal(t, "16123", port, "the reused daemon's own port")
			} else {
				assert.Equal(t, int32(1), starts.Load(), "an outdated daemon was kept")
				assert.Equal(t, "17000", port, "the new daemon's port")
			}
		})
	}
}

// A daemon this package starts records the protocol it speaks, so the next
// tool can decide by it.
func TestAStartedDaemonRecordsItsProtocol(t *testing.T) {
	d := newTestDaemon(t)
	require.NoError(t, os.MkdirAll(d.Store.Dir(), 0o755))
	require.NoError(t, d.writeRecord(os.Getpid(), "16123"))

	rec, err := d.readRecord()
	require.NoError(t, err)
	assert.Equal(t, DaemonProtocol, rec.Protocol)
}
