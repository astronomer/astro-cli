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
// Changes a client cannot observe do not bump it: a change to what the daemon
// serves (its pages, the Airflow theme, a routing fix) bumps DaemonRevision
// instead, and its logging bumps neither.
//
// It is deliberately not the host's version. Two tools built at different
// times share one daemon — Astro Desktop's bundled astro and a user's own CLI
// — and comparing their versions would have each replace the other's daemon
// on every start. Protocol 0 is any daemon from before this field existed.
const DaemonProtocol = 1

// DaemonRevision orders daemons that speak the same DaemonProtocol by what
// they serve, so an astro that serves something newer replaces a running
// daemon that serves something older: a new Airflow theme or landing page is
// on screen after an upgrade, not after the old daemon happens to exit.
//
// Bump it with any change a user would see from the daemon: the pages it
// renders, what it injects into Airflow's, how it routes. Never lower it.
// TestDaemonRevisionCoversWhatItServes fails when the pages or the theme change
// without a bump, since those are the changes nothing else would flag.
//
// It is a number rather than the host's version or build time for the reason
// DaemonProtocol is: tools built at different times share one daemon, and only
// an order every one of them agrees on lets the older tool adopt the newer
// daemon instead of the two replacing each other. Revision 0 is any daemon
// from before this field existed.
const DaemonRevision = 1

// ServeSubcommand is the astro CLI's hidden subcommand that runs the daemon's
// server loop: a host spawns `astro ServeSubcommand --port <port>`. Every host
// names it from here, because it is part of how one tool recognizes another's
// daemon (see processLooksLikeProxy): a host spelling it differently would
// spawn a binary that refuses the command, and stop recognizing the daemons
// the others started.
const ServeSubcommand = "__proxy-serve"

// Daemon is the proxy run as a background process of its own: one per route
// store, started by whichever tool needs it first and adopted by the rest.
//
// It lives here rather than in either tool because both have to agree on all
// of it — which files name a running daemon, when one is reused, how it is
// stopped, and what it does about an astro 1.x proxy holding its port. A host
// supplies only what differs between them: the executable that serves (the CLI
// re-execs itself; another host points at the astro binary it ships).
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
	// exitPollWait is how often waitForExit looks for a stopped process.
	exitPollWait = 25 * time.Millisecond
	// startTimeout plus two stops (stopTimeout and a poll each) must stay
	// below the routes lock timeout (lockTimeout): EnsureRunning holds that
	// lock while it stops an older daemon, takes the port from an astro 1.x
	// proxy (takeOverFromV1, another stop) and starts, and waiters on it must
	// outlast a worst-case start.
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
//
// The protocol and revision are the starter's own, which stand for Exe's
// because every host builds the two from one commit: the CLI runs itself, and
// the desktop bundles the astro built from the CLI commit it links.
func (d *Daemon) writeRecord(pid int, port string) error {
	return WriteRecord(d.RecordPath(), Record{
		PID:      pid,
		Version:  d.Version,
		Port:     port,
		Protocol: DaemonProtocol,
		Revision: DaemonRevision,
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
	resp, _, ok := probeProxy(port, "localhost")
	return ok && resp.Header.Get(SignatureHeader) == SignatureValue
}

// probeProxy GETs / on port with the given Host, within proxyProbeTimeout, and
// returns the response with the start of its body (up to probeBodyLimit).
func probeProxy(port, host string) (resp *http.Response, body []byte, ok bool) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/", http.NoBody)
	if err != nil {
		return nil, nil, false
	}
	req.Host = host

	client := &http.Client{Timeout: proxyProbeTimeout}
	resp, err = client.Do(req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, probeBodyLimit))
	return resp, body, err == nil
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
// A running daemon is reused when it is this one or newer, by DaemonProtocol
// and then DaemonRevision, and replaced when it is older (see reusable). Before a
// start, an astro 1.x proxy holding the port is stopped (takeOverFromV1), so the
// daemon serves on port rather than on a fallback whichever tool starts it.
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
		// kill-0 only proves *some* process owns this PID. A SIGKILL'd daemon,
		// a crash or a reboot can leave a record whose PID an unrelated process
		// later recycles. Confirm the process is actually our proxy before
		// trusting the record, and before replacing it too: replacing signals
		// the PID, and an unconfirmed one may be anybody's.
		switch {
		case !isProxyDaemon(d, rec.PID, rec.Port):
			d.logf("PID %d is alive but is not the proxy daemon; treating the record as stale and restarting", rec.PID)
		case reusable(rec):
			d.logf("proxy daemon already running (PID %d, version %q)", rec.PID, rec.Version)
			return rec.Port, nil
		default:
			d.logf("proxy daemon (PID %d, version %q) is protocol %d revision %d, older than %d/%d; restarting",
				rec.PID, rec.Version, rec.Protocol, rec.Revision, DaemonProtocol, DaemonRevision)
			d.Stop() //nolint:errcheck // Stop reports nothing a start could act on
		}
	}

	// Clean up a stale record
	os.Remove(d.RecordPath()) //nolint:errcheck // best-effort cleanup

	beforeStart(d, port)
	return d.Start(port)
}

// beforeStart is what EnsureRunning does just before a start, under the routes
// lock and with the port about to be bound. A seam so a test can see that it
// runs before a start and not when a daemon is adopted.
var beforeStart = (*Daemon).takeOverFromV1

// reusable reports whether a running daemon that wrote rec can serve for this
// one: it speaks a newer DaemonProtocol, or this one at this DaemonRevision or
// a newer one.
//
// Newer is kept, not replaced, because a newer protocol only ever adds to what
// an older client relies on, and a newer revision serves what this one would
// or later. Without that, two tools built at different times (the desktop's
// bundled astro and an installed CLI) would each replace the other's daemon on
// every start, dropping whatever it was serving. The cost is a downgrade: the
// newer daemon stays until its last route is gone or it is stopped.
func reusable(rec Record) bool {
	if rec.Protocol != DaemonProtocol {
		return rec.Protocol > DaemonProtocol
	}
	return rec.Revision >= DaemonRevision
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
		cmd.Process.Kill() //nolint:errcheck // the start has already failed
		cmd.Wait()         //nolint:errcheck // reaps the child; its exit status is the kill
		return "", fmt.Errorf("proxy daemon did not start: %w (see %s)", err, d.LogPath())
	}

	if err := d.writeRecord(pid, bound); err != nil {
		if requestStop(pid) {
			forgetStop(pid)
		}
		reapWhenItExits(cmd)
		return "", fmt.Errorf("error writing proxy PID file: %w", err)
	}

	reapWhenItExits(cmd)

	d.logf("proxy daemon started (PID %d) on port %s", pid, bound)
	return bound, nil
}

// reapWhenItExits waits for the daemon in the background, so whoever started
// it collects its exit status whenever it ends.
//
// Not Release. A starter that outlives its daemon is its parent until it
// exits, and on unix a child that exits unwaited stays a zombie, which
// IsPIDAlive's signal 0 still reports alive: a Stop from that starter would
// wait out the whole stopTimeout and then SIGKILL the corpse, and every start
// and stop would leave one more <defunct> process behind. A host that starts
// the daemon and keeps running for hours hits this; the CLI never saw it because
// its process exits right after the start and init collects the orphan. On Windows the
// wait holds the process handle until the daemon exits and then closes it, as
// Release would have closed it at once.
func reapWhenItExits(cmd *exec.Cmd) {
	go cmd.Wait() //nolint:errcheck // the exit status is nobody's business but the log's
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
		forgetStop(pid)
	}
	d.removeFiles()
	return nil
}

// waitForExit polls until pid is gone, escalating to killProcess once
// stopTimeout has passed. Shared with StopIfEmpty, which does its own signaling under the
// routes lock and then waits out here.
//
// It looks before it sleeps, and then every exitPollWait: a daemon with nothing
// in flight exits within a few milliseconds of being asked, and a host that
// stops it on the way out of a project (Astro Desktop removing its last route)
// waits for exactly that long rather than a fixed half second.
func waitForExit(pid int) {
	deadline := time.Now().Add(stopTimeout)
	for {
		if !IsPIDAlive(pid) {
			return
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(exitPollWait)
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
	forgetStop(pid)
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
