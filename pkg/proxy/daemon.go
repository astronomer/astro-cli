package proxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	daemonRecordName = "proxy.pid"
	daemonLogName    = "proxy.log"
)

// DaemonProtocol is the version of the contract between the daemon and the
// tools that start, adopt and stop it, recorded in the daemon's Record. A
// running daemon is reused by a tool whose protocol is the same or older, and
// replaced by one whose protocol is newer.
//
// It covers what a client relies on across processes: the routes.json schema,
// the record's format, and any daemon behavior a client depends on (the
// signature header, the files it writes, how it stops). Bump it when a change
// to any of those would leave an older daemon serving a newer client wrongly.
// Changes a client cannot observe do not bump it — the pages the daemon
// renders, its logging, a fix that leaves those contracts as they were.
//
// It is deliberately not the host's version. Two tools built at different
// times share one daemon — Astro Desktop's bundled astro and a user's own CLI
// — and comparing their versions would have each replace the other's daemon
// on every start. Protocol 0 is any daemon from before this field existed.
const DaemonProtocol = 1

// Daemon is the proxy run as a background process of its own: one per route
// store, started by whichever tool needs it first and adopted by the rest.
//
// It lives here rather than in either tool because both have to agree on all
// of it — which files name a running daemon, when one is reused, how it is
// stopped. A host supplies only what differs between them: the executable that
// serves (the CLI re-execs itself; another host points at the astro binary it
// ships), and anything it must do before a start.
type Daemon struct {
	// Store is the route store the daemon serves. Its directory also holds the
	// daemon's own files: the record, the bound port and the log.
	Store *Store

	// Exe and ServeArgs are what Start runs, with "--port <port>" appended:
	// for the CLI, its own binary and its hidden serve subcommand. ServeArgs
	// also identifies a daemon by its command line when its port cannot be
	// asked (see processLooksLikeProxy).
	Exe       string
	ServeArgs []string

	// Version is the host's version, written into the record for diagnostics.
	// It does not decide reuse; DaemonProtocol does.
	Version string

	// BeforeStart runs under the routes lock just before a start, with the
	// port about to be bound. The CLI stops an astro 1.x proxy holding that
	// port here, so the daemon serves there rather than on a fallback. Nil
	// does nothing.
	BeforeStart func(port string)

	// Logf receives debug messages about the daemon's lifecycle. Nil discards
	// them.
	Logf func(format string, args ...any)
}

// RecordPath is where the running daemon's record lives (proxy.pid).
func (d *Daemon) RecordPath() string { return filepath.Join(d.Store.Dir(), daemonRecordName) }

// LogPath is where the daemon's output goes (proxy.log).
func (d *Daemon) LogPath() string { return filepath.Join(d.Store.Dir(), daemonLogName) }

// readRecord reads this daemon's record.
//
// The format is Record's, because it is a contract between this daemon and
// whoever else needs to find a running proxy — the desktop publishes a record
// of its own in the same shape. Parsing it anywhere else as well is how two
// readers come to disagree about a field.
//
// It returns the record rather than its fields: unpacking them here only to
// thread them through every caller means each field the contract gains costs
// another return value and an edit at each one.
func (d *Daemon) readRecord() (Record, error) {
	return ReadRecord(d.RecordPath())
}

// IsRunning reports the PID the record names and whether that process is
// alive.
func (d *Daemon) IsRunning() (int, bool) {
	r, err := d.readRecord()
	if err != nil {
		return 0, false
	}
	// Not LiveRecord: this reports the PID it found even when that process is
	// gone, which is what a caller cleaning up a stale record needs.
	return r.PID, IsPIDAlive(r.PID)
}

// BoundPort returns the port the running daemon reported at bind time, or ""
// when unknown (daemon not running, or started by a build that didn't record
// it).
func (d *Daemon) BoundPort() string {
	r, ok := LiveRecord(d.RecordPath())
	if !ok {
		return ""
	}
	return r.Port
}

const (
	daemonPortName = "proxy.port"
	// daemonFallbackName holds the port the daemon bound the last time its own
	// was taken. Unlike proxy.port it outlives the daemon.
	daemonFallbackName = "proxy.fallback-port"
	stopTimeout        = 5 * time.Second
	stopPollWait       = 500 * time.Millisecond
	// startTimeout plus two stops (stopTimeout and a poll each) must stay
	// below the routes lock timeout (lockTimeout): EnsureRunning holds that
	// lock while it stops an older daemon, runs BeforeStart (the CLI takes the
	// port from astro 1.x there) and starts, and waiters on it must outlast a
	// worst-case start.
	startTimeout  = 10 * time.Second
	startPollWait = 50 * time.Millisecond

	// proxyProbeTimeout bounds the liveness probe so EnsureRunning can't hang
	// on a recycled PID that holds an open but unresponsive socket.
	proxyProbeTimeout = 500 * time.Millisecond
	// probeBodyLimit caps how much of a probe's response body is read.
	probeBodyLimit = 64 << 10
)

func (d *Daemon) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

// portPath is where the daemon writes its bound port once listening; Start
// waits for it.
func (d *Daemon) portPath() string { return filepath.Join(d.Store.Dir(), daemonPortName) }

func (d *Daemon) fallbackPath() string { return filepath.Join(d.Store.Dir(), daemonFallbackName) }

// writeRecord publishes this daemon's record. See WriteRecord for the format.
func (d *Daemon) writeRecord(pid int, port string) error {
	return WriteRecord(d.RecordPath(), Record{
		PID:      pid,
		Version:  d.Version,
		Port:     port,
		Protocol: DaemonProtocol,
	})
}

// removeFiles drops the record and the bound-port file, which together are
// what makes a daemon adoptable.
func (d *Daemon) removeFiles() {
	os.Remove(d.RecordPath()) //nolint:errcheck // best-effort cleanup
	os.Remove(d.portPath())   //nolint:errcheck // best-effort cleanup
}

// isProxyDaemon reports whether the live process at pid is really the proxy
// daemon rather than an unrelated process that recycled a stale PID. It first
// asks the recorded port for the proxy's HTTP signature; if that can't be
// reached (no recorded port, or the request fails) it falls back to matching
// the process's command line.
var isProxyDaemon = func(d *Daemon, pid int, port string) bool {
	if port != "" && probeProxySignature(port) {
		return true
	}
	return processLooksLikeProxy(d, pid)
}

// probeProxySignature does a short-timeout GET against port and reports
// whether the response carries the proxy's signature header. A bare loopback
// Host makes the proxy answer with its own landing page (which sets the
// header) instead of routing to a backend.
func probeProxySignature(port string) bool {
	resp, ok := probeProxy(port, "localhost")
	return ok && resp.Header.Get(SignatureHeader) == SignatureValue
}

// probeProxy GETs / on port with the given Host, within proxyProbeTimeout.
func probeProxy(port, host string) (*http.Response, bool) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/", http.NoBody)
	if err != nil {
		return nil, false
	}
	req.Host = host

	client := &http.Client{Timeout: proxyProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	// Drained so the connection closes cleanly; the body itself says nothing
	// the header does not.
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, probeBodyLimit))
	return resp, err == nil
}

// startProxy starts a proxy on port. When port is taken, it tries the port it
// fell back to last time before any other, and remembers a new one, so a
// project's URL does not change on every restart while something else holds
// port.
func (d *Daemon) startProxy(port string) (*Proxy, error) {
	p := NewProxy(port, d.Store)
	if last, err := os.ReadFile(d.fallbackPath()); err == nil {
		p.FallbackPort = strings.TrimSpace(string(last))
	}
	if err := p.Start(); err != nil {
		return nil, err
	}
	if p.Port() != port && p.Port() != p.FallbackPort {
		if err := os.WriteFile(d.fallbackPath(), []byte(p.Port()), FilePermRW); err != nil {
			d.logf("could not remember fallback port %s: %s", p.Port(), err)
		}
	}
	return p, nil
}

// EnsureRunning starts the daemon if it's not already running, and returns the
// port it is actually listening on.
//
// A running daemon is reused when it speaks this DaemonProtocol or a newer
// one, and replaced when it speaks an older one (see reusable). Before a
// start, BeforeStart runs with the port about to be bound.
func (d *Daemon) EnsureRunning(port string) (string, error) {
	if port == "" {
		port = DefaultPort
	}

	// Hold the routes lock across check-then-start: without it two tools
	// starting at once both see "not running" and each spawn a daemon.
	lockFile, err := d.Store.AcquireLock()
	if err != nil {
		return "", fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer ReleaseLock(lockFile)

	rec, err := d.readRecord()
	if err == nil && IsPIDAlive(rec.PID) {
		if reusable(rec) {
			// kill-0 only proves *some* process owns this PID. A SIGKILL'd
			// daemon can leave a record whose PID an unrelated process later
			// recycles. Confirm the process is actually our proxy before
			// trusting the record.
			if isProxyDaemon(d, rec.PID, rec.Port) {
				d.logf("proxy daemon already running (PID %d, version %q)", rec.PID, rec.Version)
				return rec.Port, nil
			}
			d.logf("PID %d is alive but is not the proxy daemon; treating the record as stale and restarting", rec.PID)
		} else {
			d.logf("proxy daemon (PID %d, version %q) speaks protocol %d, older than %d; restarting",
				rec.PID, rec.Version, rec.Protocol, DaemonProtocol)
			d.Stop() //nolint:errcheck // Stop reports nothing a start could act on
		}
	}

	// Clean up a stale record
	os.Remove(d.RecordPath()) //nolint:errcheck // best-effort cleanup

	if d.BeforeStart != nil {
		d.BeforeStart(port)
	}
	return d.Start(port)
}

// reusable reports whether a running daemon that wrote rec can serve for this
// one: it speaks this DaemonProtocol or a newer one.
//
// Newer is kept, not replaced, because a newer protocol only ever adds to what
// an older client relies on. Without that, two tools built at different times
// (the desktop's bundled astro and an installed CLI) would each replace the
// other's daemon on every start, dropping whatever it was serving.
func reusable(rec Record) bool {
	return rec.Protocol >= DaemonProtocol
}

// Start runs Exe with ServeArgs in the background. It waits for the daemon to
// bind before writing the record, so a record always names a daemon that owns
// a socket, and returns the port the daemon actually bound (which can differ
// from the requested one when it was taken).
//
// Start does not check for a running daemon or take the routes lock;
// EnsureRunning is the entry point that does both.
func (d *Daemon) Start(port string) (string, error) {
	return startDaemon(d, port)
}

// startDaemon is Start's body, a seam so tests never spawn a process.
var startDaemon = func(d *Daemon, port string) (string, error) {
	if d.Exe == "" {
		return "", errors.New("no executable to run the proxy daemon with")
	}
	if err := os.MkdirAll(d.Store.Dir(), DirPermRWX); err != nil {
		return "", fmt.Errorf("error creating proxy directory: %w", err)
	}

	// Drop any port file from a previous daemon so the wait below can't
	// read a stale port.
	os.Remove(d.portPath()) //nolint:errcheck // best-effort cleanup

	logFile, err := os.OpenFile(d.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, FilePermRW)
	if err != nil {
		return "", fmt.Errorf("error opening proxy log file: %w", err)
	}
	defer logFile.Close()

	args := append(append([]string{}, d.ServeArgs...), "--port", port)
	cmd, err := startDetached(func() *exec.Cmd {
		c := exec.Command(d.Exe, args...) //nolint:gosec // the host names its own executable
		c.Stdout = logFile
		c.Stderr = logFile
		// Detach from parent process — don't pass stdin
		c.Stdin = nil
		return c
	})
	if err != nil {
		return "", fmt.Errorf("error starting proxy daemon: %w", err)
	}
	pid := cmd.Process.Pid

	bound, err := d.waitForPortFile()
	if err != nil {
		killProcess(pid)
		cmd.Process.Release() //nolint:errcheck // the start has already failed
		return "", fmt.Errorf("proxy daemon did not start: %w (see %s)", err, d.LogPath())
	}

	if err := d.writeRecord(pid, bound); err != nil {
		requestStop(pid)
		cmd.Process.Release() //nolint:errcheck // the start has already failed
		return "", fmt.Errorf("error writing proxy PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	cmd.Process.Release() //nolint:errcheck // nothing to do if it fails

	d.logf("proxy daemon started (PID %d) on port %s", pid, bound)
	return bound, nil
}

// waitForPortFile polls for the port file the daemon writes after binding.
func (d *Daemon) waitForPortFile() (string, error) {
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(d.portPath())
		if err == nil {
			if port := strings.TrimSpace(string(data)); port != "" {
				return port, nil
			}
		}
		time.Sleep(startPollWait)
	}
	return "", fmt.Errorf("timed out waiting for proxy daemon to bind")
}

// Serve runs the proxy server in the foreground until asked to stop (see
// stopRequests). It is what the daemon process runs: the body of the command
// Start spawns. Once listening it writes the bound port to the port file,
// which is what Start waits for before writing the record.
//
// It listens for a stop before it writes that file, because the file is what
// makes the daemon adoptable: a stop can be sent the moment a record names
// this process, and one sent before anything listens would end it without
// the graceful shutdown.
func (d *Daemon) Serve(port string) error {
	p, err := d.startProxy(port)
	if err != nil {
		return err
	}

	stop, release, err := stopRequests()
	if err != nil {
		p.Stop()
		return fmt.Errorf("error listening for a stop request: %w", err)
	}
	defer release()

	if err := os.WriteFile(d.portPath(), []byte(p.Port()), FilePermRW); err != nil {
		p.Stop()
		return fmt.Errorf("error writing proxy port file: %w", err)
	}

	d.logf("proxy serving on port %s", p.Port())
	<-stop
	d.logf("proxy received shutdown signal")
	p.Stop()
	return nil
}

// Stop asks the daemon to stop (requestStop), then forces it (killProcess)
// if it has not exited within stopTimeout.
func (d *Daemon) Stop() error {
	pid, alive := d.IsRunning()
	if !alive {
		if pid > 0 {
			d.removeFiles()
		}
		return nil
	}

	d.logf("stopping proxy daemon (PID %d)", pid)
	if requestStop(pid) {
		waitForDaemonExit(pid)
	}
	d.removeFiles()
	return nil
}

// waitForExit polls until pid is gone, escalating to killProcess once
// stopTimeout has passed. Shared with StopIfEmpty, which does its own signaling under the
// routes lock and then waits out here.
func waitForExit(pid int) {
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollWait)
		if !IsPIDAlive(pid) {
			return
		}
	}
	killProcess(pid)
	time.Sleep(stopPollWait)
}

// StopIfEmpty stops the daemon when no route needs it any more.
//
// Split across the routes lock rather than wrapped in it. What has to be
// atomic is the decision and the moment the daemon stops being adoptable:
// EnsureRunning takes this same lock and adopts whatever the record names, so
// once that record is gone under the lock, no start can attach itself to a
// daemon that is already being signaled. That is the whole of the race, and it
// is closed by the ordering rather than by the duration of the hold.
//
// The waiting happens outside. Holding the lock across it blocks everything
// else that needs the lock for as long as the daemon takes to exit — including
// the daemon's own shutdown if anything it is serving takes the lock — so a
// clean stop could reach the forced kill in waitForExit with a dropped connection.
// Holding it for the full stopTimeout also lets a stop and a start queue past
// the routes lock timeout, so a third waiter's AddRoute fails and that project
// starts with no hostname at all.
func (d *Daemon) StopIfEmpty() {
	pid, ok := d.claimForStop()
	if !ok {
		return
	}
	waitForDaemonExit(pid)
}

// waitForDaemonExit is the seam both stops wait through, so a test can observe
// what is true while the wait is in flight — which for StopIfEmpty is the
// claim, and is not visible from either side of the call — and so a test that
// replaces a daemon does not wait out stopTimeout on a pid nothing owns.
var waitForDaemonExit = waitForExit

// claimForStop decides, under the routes lock, whether the daemon should go;
// if so it makes the daemon unadoptable and signals it, returning the pid
// still to be reaped.
//
// Every caller has already worked out that no route remains — the engines'
// RemoveRoute paths call this only when their own count reaches zero, and they
// count through a store that judges a route by its owning record. The count
// here is a second, weaker opinion: the default predicate judges a route by
// the pid recorded in it, which calls a route dead whenever the process that
// registered it has been replaced. It can therefore only ever agree with a
// caller that has already seen zero, which is why it is kept as a guard and
// must never become the only gate.
//
// Pruned to decide, never written back. ListRoutes persists what it prunes,
// and persisting that predicate's verdict is what deletes a live project's
// route. Stale rows are for a sweep that knows better to remove (the CLI's
// `astro local list --clean`).
func (d *Daemon) claimForStop() (int, bool) {
	lockFile, err := d.Store.AcquireLock()
	if err != nil {
		// Worth saying: the daemon is now never reaped, and nothing else will
		// mention it.
		d.logf("not stopping the proxy daemon: %s", err)
		return 0, false
	}
	defer ReleaseLock(lockFile)

	// ReadRoutes, not ListRoutes: ListRoutes takes this same lock, and the
	// flock is not reentrant across descriptors — measured. It would not
	// deadlock, because AcquireLock polls LOCK_NB against a deadline; it would
	// stall until the lock timeout and then fail, and the daemon would simply
	// never be stopped, slowly.
	routes, err := d.Store.ReadRoutes()
	if err != nil {
		d.logf("not stopping the proxy daemon: %s", err)
		return 0, false
	}
	if len(PruneStaleRoutes(routes)) != 0 {
		return 0, false
	}

	pid, alive := d.IsRunning()
	if !alive {
		if pid > 0 {
			d.removeFiles()
		}
		return 0, false
	}

	d.logf("no active routes, stopping proxy daemon (PID %d)", pid)
	// The record first, and under the lock, because that is what closes the
	// race: a start holding this lock next finds nothing to adopt and brings
	// up a daemon of its own.
	d.removeFiles()
	return pid, requestStop(pid)
}
