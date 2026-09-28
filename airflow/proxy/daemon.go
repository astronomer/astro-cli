//go:build !windows

package proxy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	semver "github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

const (
	pidFileName  = "proxy.pid"
	logFileName  = "proxy.log"
	portFileName = "proxy.port"
	// fallbackFileName holds the port the daemon bound the last time its own
	// was taken. Unlike proxy.port it outlives the daemon.
	fallbackFileName = "proxy.fallback-port"
	stopTimeout      = 5 * time.Second
	stopPollWait     = 500 * time.Millisecond
	// startTimeout plus two stops (stopTimeout and a poll each) must stay
	// below pkg/proxy's routes lock timeout (30s): EnsureRunning holds that
	// lock while it stops an older v2 daemon, takes the port from astro 1.x
	// and starts, and waiters on it must outlast a worst-case start.
	startTimeout  = 10 * time.Second
	startPollWait = 50 * time.Millisecond
)

// pidFilePath returns the path to ~/.astro/proxy/proxy.pid.
func pidFilePath() string {
	return filepath.Join(Routes().Dir(), pidFileName)
}

// logFilePath returns the path to ~/.astro/proxy/proxy.log.
func logFilePath() string {
	return filepath.Join(Routes().Dir(), logFileName)
}

// portFilePath returns the path to ~/.astro/proxy/proxy.port. The daemon
// writes its bound port here once listening; StartDaemon waits for it.
func portFilePath() string {
	return filepath.Join(Routes().Dir(), portFileName)
}

// fallbackFilePath returns the path to ~/.astro/proxy/proxy.fallback-port.
func fallbackFilePath() string {
	return filepath.Join(Routes().Dir(), fallbackFileName)
}

// parsePIDFile reads this daemon's record.
//
// The format is pkg/proxy's, because it is a contract between this daemon and
// whoever else needs to find a running proxy — the desktop publishes a record
// of its own in the same shape. Parsing it here as well is how the two come to
// disagree about a field.
//
// It returns the record rather than its fields: unpacking three values here
// only to thread them through every caller means each field the contract gains
// costs another return value and an edit at each one.
func parsePIDFile() (pkgproxy.Record, error) {
	return pkgproxy.ReadRecord(pidFilePath())
}

// writePIDFile publishes this daemon's record. See pkg/proxy.WriteRecord for
// the format and why it lives there.
func writePIDFile(pid int, port string) error {
	return pkgproxy.WriteRecord(pidFilePath(), pkgproxy.Record{
		PID:     pid,
		Version: version.CurrVersion,
		Port:    port,
	})
}

// IsRunning checks if the proxy daemon is running by reading the PID file
// and verifying the process is alive.
func IsRunning() (int, bool) {
	r, err := parsePIDFile()
	if err != nil {
		return 0, false
	}
	// Not LiveRecord: this reports the PID it found even when that process is
	// gone, which is what a caller cleaning up a stale record needs.
	return r.PID, pkgproxy.IsPIDAlive(r.PID)
}

// BoundPort returns the port the running daemon reported at bind time, or ""
// when unknown (daemon not running, or started by an older CLI that didn't
// record it).
func BoundPort() string {
	r, ok := pkgproxy.LiveRecord(pidFilePath())
	if !ok {
		return ""
	}
	return r.Port
}

// EnsureRunning starts the proxy daemon if it's not already running.
// If the running daemon was started by an older or unrelated CLI version, it
// is restarted to avoid incompatibilities with route file formats; one from a
// newer release is kept, since it reads every field this CLI writes.
// Before a start, an astro 1.x proxy holding port is stopped (see
// takeOverFromV1), so the daemon serves on port rather than a fallback.
// Returns the port the proxy is actually listening on.
func EnsureRunning(port string) (string, error) {
	if port == "" {
		port = pkgproxy.DefaultPort
	}

	// Hold the routes lock across check-then-start: without it two CLIs
	// starting at once both see "not running" and each spawn a daemon.
	lockFile, err := Routes().AcquireLock()
	if err != nil {
		return "", fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer pkgproxy.ReleaseLock(lockFile)

	rec, err := parsePIDFile()
	pid, ver, bound := rec.PID, rec.Version, rec.Port
	if err == nil && pkgproxy.IsPIDAlive(pid) {
		switch {
		case ver == version.CurrVersion || version.CurrVersion == "" || isNewerVersion(ver, version.CurrVersion):
			// kill-0 only proves *some* process owns this PID. A SIGKILL'd
			// daemon can leave a PID file whose PID an unrelated process later
			// recycles — most likely on dev builds, where an empty version
			// can't force the mismatch restart below. Confirm the process is
			// actually our proxy before trusting the file.
			if isProxyDaemon(pid, bound) {
				logger.Debugf("proxy daemon already running (PID %d)", pid)
				if bound != "" {
					return bound, nil
				}
				return port, nil // older daemon didn't record its port
			}
			logger.Debugf("PID %d is alive but is not the proxy daemon; treating PID file as stale and restarting", pid)
		default:
			// Version mismatch — restart the daemon
			logger.Debugf("proxy daemon version %q doesn't match CLI version %q, restarting", ver, version.CurrVersion)
			StopDaemon() //nolint:errcheck // error deliberately ignored in this v1 path
		}
	}

	// Clean up stale PID file
	os.Remove(pidFilePath()) //nolint:errcheck // best-effort cleanup

	takeOverFromV1(port)
	return StartDaemon(port)
}

// isNewerVersion reports whether recorded is a later release than current in
// the same major version.
// A version that is not semver, such as a SNAPSHOT build's, is never newer.
func isNewerVersion(recorded, current string) bool {
	r, err := semver.StrictNewVersion(strings.TrimPrefix(recorded, "v"))
	if err != nil {
		return false
	}
	c, err := semver.StrictNewVersion(strings.TrimPrefix(current, "v"))
	if err != nil {
		return false
	}
	return r.Major() == c.Major() && r.GreaterThan(c)
}

// proxyProbeTimeout bounds the liveness probe so EnsureRunning can't hang on a
// recycled PID that holds an open but unresponsive socket.
const proxyProbeTimeout = 500 * time.Millisecond

// isProxyDaemon reports whether the live process at pid is really this CLI's
// proxy daemon rather than an unrelated process that recycled a stale PID. It
// first asks the recorded port for the proxy's HTTP signature; if that can't be
// reached (no recorded port, or the request fails) it falls back to matching
// the process's command line.
var isProxyDaemon = func(pid int, port string) bool {
	if port != "" && probeProxySignature(port) {
		return true
	}
	return processLooksLikeProxy(pid)
}

// probeProxySignature does a short-timeout GET against the recorded port and
// reports whether the response carries the proxy's signature header. A bare
// loopback Host makes the proxy answer with its own landing page (which sets
// the header) instead of routing to a backend.
func probeProxySignature(port string) bool {
	resp, _, ok := probeProxy(port, "localhost")
	return ok && resp.Header.Get(pkgproxy.SignatureHeader) == pkgproxy.SignatureValue
}

// probeBodyLimit caps how much of a probe's response body is read.
const probeBodyLimit = 64 << 10

// probeProxy GETs / on port with the given Host, within proxyProbeTimeout, and
// returns the response with the start of its body.
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

// processLooksLikeProxy reports whether pid's command line looks like this
// CLI's proxy daemon, which re-execs itself with ServeSubcommand. It's the
// fallback for daemons that didn't record a port (older CLIs).
var processLooksLikeProxy = func(pid int) bool {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // reviewed; not a new risk in this v1 code
	if err != nil {
		return false
	}
	return strings.Contains(string(out), ServeSubcommand)
}

// StartDaemon starts the proxy as a background process by re-executing the
// current CLI binary with a hidden subcommand. It waits for the daemon to
// bind before writing the PID file, so a PID file always names a daemon that
// owns a socket, and returns the port the daemon actually bound (which can
// differ from the requested one when it was taken).
var StartDaemon = func(port string) (string, error) {
	if err := os.MkdirAll(Routes().Dir(), pkgproxy.DirPermRWX); err != nil {
		return "", fmt.Errorf("error creating proxy directory: %w", err)
	}

	// Drop any port file from a previous daemon so the wait below can't
	// read a stale port.
	os.Remove(portFilePath()) //nolint:errcheck // best-effort cleanup

	logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, pkgproxy.FilePermRW)
	if err != nil {
		return "", fmt.Errorf("error opening proxy log file: %w", err)
	}
	defer logFile.Close()

	// Find the current CLI binary
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error finding CLI executable: %w", err)
	}

	cmd := exec.Command(exe, ServeSubcommand, "--port", port)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	// Detach from parent process — don't pass stdin
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("error starting proxy daemon: %w", err)
	}
	pid := cmd.Process.Pid

	bound, err := waitForPortFile()
	if err != nil {
		syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // error deliberately ignored in this v1 path
		cmd.Process.Release()              //nolint:errcheck // error deliberately ignored in this v1 path
		return "", fmt.Errorf("proxy daemon did not start: %w (see %s)", err, logFilePath())
	}

	if err := writePIDFile(pid, bound); err != nil {
		syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // error deliberately ignored in this v1 path
		cmd.Process.Release()              //nolint:errcheck // error deliberately ignored in this v1 path
		return "", fmt.Errorf("error writing proxy PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	cmd.Process.Release() //nolint:errcheck // error deliberately ignored in this v1 path

	logger.Debugf("proxy daemon started (PID %d) on port %s", pid, bound)
	return bound, nil
}

// waitForPortFile polls for the port file the daemon writes after binding.
func waitForPortFile() (string, error) {
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(portFilePath())
		if err == nil {
			if port := strings.TrimSpace(string(data)); port != "" {
				return port, nil
			}
		}
		time.Sleep(startPollWait)
	}
	return "", fmt.Errorf("timed out waiting for proxy daemon to bind")
}

// Serve runs the proxy server in the foreground until SIGTERM/SIGINT. It is
// the body of the hidden ServeSubcommand the daemon runs as.
// Once listening it writes the bound port to the port file, which is what
// StartDaemon waits for before writing the PID file.
func Serve(port string) error {
	p, err := startProxy(port)
	if err != nil {
		return err
	}

	if err := os.WriteFile(portFilePath(), []byte(p.Port()), pkgproxy.FilePermRW); err != nil {
		p.Stop()
		return fmt.Errorf("error writing proxy port file: %w", err)
	}

	logger.Debugf("proxy serving on port %s", p.Port())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	logger.Debugf("proxy received shutdown signal")
	p.Stop()
	return nil
}

// startProxy starts a proxy on port. When port is taken, it tries the port it
// fell back to last time before any other, and remembers a new one, so a
// project's URL does not change on every restart while something else holds
// port.
func startProxy(port string) (*pkgproxy.Proxy, error) {
	p := pkgproxy.NewProxy(port, Routes())
	if last, err := os.ReadFile(fallbackFilePath()); err == nil {
		p.FallbackPort = strings.TrimSpace(string(last))
	}
	if err := p.Start(); err != nil {
		return nil, err
	}
	if p.Port() != port && p.Port() != p.FallbackPort {
		if err := os.WriteFile(fallbackFilePath(), []byte(p.Port()), pkgproxy.FilePermRW); err != nil {
			logger.Debugf("could not remember fallback port %s: %s", p.Port(), err)
		}
	}
	return p, nil
}

// StopDaemon stops the proxy daemon by sending SIGTERM, then SIGKILL if needed.
func StopDaemon() error {
	pid, alive := IsRunning()
	if !alive {
		if pid > 0 {
			removeDaemonFiles()
		}
		return nil
	}

	logger.Debugf("stopping proxy daemon (PID %d)", pid)
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // error deliberately ignored in this v1 path
	waitForExit(pid)
	removeDaemonFiles()
	return nil
}

// waitForExit polls until pid is gone, escalating to SIGKILL once stopTimeout
// has passed. Shared with StopIfEmpty, which does its own signaling under the
// routes lock and then waits out here.
func waitForExit(pid int) {
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollWait)
		if !pkgproxy.IsPIDAlive(pid) {
			return
		}
	}
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // error deliberately ignored in this v1 path
	time.Sleep(stopPollWait)
}

func removeDaemonFiles() {
	os.Remove(pidFilePath())  //nolint:errcheck // best-effort cleanup
	os.Remove(portFilePath()) //nolint:errcheck // best-effort cleanup
}

// StopIfEmpty stops the proxy daemon when no route needs it any more.
//
// Split across the routes lock rather than wrapped in it. What has to be
// atomic is the decision and the moment the daemon stops being adoptable:
// EnsureRunning takes this same lock and adopts whatever the pid record names,
// so once that record is gone under the lock, no start can attach itself to a
// daemon that is already being signaled. That is the whole of the race, and
// it is closed by the ordering rather than by the duration of the hold.
//
// The waiting happens outside. Holding the lock across it was the first
// attempt and it was worse than the bug: the daemon answers the landing page
// by calling ListRoutes, which takes this lock, so a stop overlapping any
// request to http://localhost:6563/ blocked the daemon's own graceful
// shutdown — srv.Shutdown's grace expired, the poll below then reached
// SIGKILL, and a clean stop became a forced kill with a dropped connection.
// Holding it for the full stopTimeout also let a stop and a start queue to
// about fifteen and a half seconds, past pkg/proxy's fifteen-second lock
// timeout, so a third waiter's AddRoute failed and that project started with
// no hostname at all.
func StopIfEmpty() {
	pid, ok := claimDaemonForStop()
	if !ok {
		return
	}
	waitForDaemonExit(pid)
}

// waitForDaemonExit is the seam the stop waits through, so a test can observe
// what is true while the wait is in flight — which for this function is the
// claim, and is not visible from either side of the call.
var waitForDaemonExit = waitForExit

// claimDaemonForStop decides, under the routes lock, whether the daemon should
// go; if so it makes the daemon unadoptable and signals it, returning the pid
// still to be reaped.
//
// Every caller has already worked out that no route remains — localshared's
// RemoveRoute and airflow/docker.go both call this only when their own count
// reaches zero, and both count through a store that judges a route by its
// owning record. The count here is a second, weaker opinion: pkgproxy's
// default predicate judges a route by the pid recorded in it, which calls a
// route dead whenever the process that registered it has been replaced. It can
// therefore only ever agree with a caller that has already seen zero, which is
// why it is kept as a guard and must never become the only gate.
//
// Pruned to decide, never written back. ListRoutes persists what it prunes,
// and persisting that predicate's verdict is what deletes a live project's
// route. Stale rows are `astro local list --clean`'s to remove, through the
// store that knows better.
func claimDaemonForStop() (int, bool) {
	store := Routes()
	lockFile, err := store.AcquireLock()
	if err != nil {
		// Worth saying: the daemon is now never reaped, and nothing else will
		// mention it.
		logger.Debugf("not stopping the proxy daemon: %s", err)
		return 0, false
	}
	defer pkgproxy.ReleaseLock(lockFile)

	// ReadRoutes, not ListRoutes: ListRoutes takes this same lock, and the
	// flock is not reentrant across descriptors — measured. It would not
	// deadlock, because AcquireLock polls LOCK_NB against a deadline; it would
	// stall until the lock timeout and then fail, and the daemon would simply never
	// be stopped, slowly.
	routes, err := store.ReadRoutes()
	if err != nil {
		logger.Debugf("not stopping the proxy daemon: %s", err)
		return 0, false
	}
	if len(pkgproxy.PruneStaleRoutes(routes)) != 0 {
		return 0, false
	}

	pid, alive := IsRunning()
	if !alive {
		if pid > 0 {
			removeDaemonFiles()
		}
		return 0, false
	}

	logger.Debugf("no active routes, stopping proxy daemon (PID %d)", pid)
	// The record first, and under the lock, because that is what closes the
	// race: a start holding this lock next finds nothing to adopt and brings
	// up a daemon of its own.
	removeDaemonFiles()
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // error deliberately ignored in this v1 path
	return pid, true
}
