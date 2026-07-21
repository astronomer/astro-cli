//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

const (
	pidFileName  = "proxy.pid"
	logFileName  = "proxy.log"
	portFileName = "proxy.port"
	stopTimeout  = 5 * time.Second
	stopPollWait = 500 * time.Millisecond
	// startTimeout must stay below pkg/proxy's routes lock timeout (15s):
	// EnsureRunning holds that lock for up to this long, and waiters on it
	// must outlast a worst-case daemon start.
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

// parsePIDFile reads the PID file and returns the PID, version, and bound port.
// The format is "<pid> <version> <port>" — version and port may be absent
// (older daemons wrote fewer fields).
func parsePIDFile() (pid int, ver, port string, err error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, "", "", err
	}

	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) == 0 {
		return 0, "", "", fmt.Errorf("empty PID file")
	}

	pid, err = strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0, "", "", fmt.Errorf("invalid PID in PID file")
	}

	rest := fields[1:]
	if len(rest) > 0 && rest[0] != "-" {
		ver = rest[0]
	}
	if len(rest) > 1 {
		port = rest[1]
	}
	return pid, ver, port, nil
}

// writePIDFile records "<pid> <version> <port>". Old CLIs read the first two
// fields and ignore the rest, so the port field is additive. "-" stands in
// for an empty version to keep the port in field three for every reader.
func writePIDFile(pid int, port string) error {
	ver := version.CurrVersion
	if ver == "" {
		ver = "-"
	}
	content := fmt.Sprintf("%d %s %s", pid, ver, port)
	return os.WriteFile(pidFilePath(), []byte(content), pkgproxy.FilePermRW)
}

// IsRunning checks if the proxy daemon is running by reading the PID file
// and verifying the process is alive.
func IsRunning() (int, bool) {
	pid, _, _, err := parsePIDFile()
	if err != nil {
		return 0, false
	}

	if !isPIDAlive(pid) {
		return pid, false
	}
	return pid, true
}

// BoundPort returns the port the running daemon reported at bind time, or ""
// when unknown (daemon not running, or started by an older CLI that didn't
// record it).
func BoundPort() string {
	pid, _, port, err := parsePIDFile()
	if err != nil || !isPIDAlive(pid) {
		return ""
	}
	return port
}

// EnsureRunning starts the proxy daemon if it's not already running.
// If the running daemon was started by a different CLI version, it is
// restarted to avoid incompatibilities with route file formats.
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

	pid, ver, bound, err := parsePIDFile()
	if err == nil && isPIDAlive(pid) {
		if ver == version.CurrVersion || version.CurrVersion == "" {
			logger.Debugf("proxy daemon already running (PID %d)", pid)
			if bound != "" {
				return bound, nil
			}
			return port, nil // older daemon didn't record its port
		}
		// Version mismatch — restart the daemon
		logger.Debugf("proxy daemon version %q doesn't match CLI version %q, restarting", ver, version.CurrVersion)
		StopDaemon() //nolint:errcheck
	}

	// Clean up stale PID file
	os.Remove(pidFilePath())

	return StartDaemon(port)
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
	os.Remove(portFilePath())

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

	cmd := exec.Command(exe, "dev", "proxy", "serve", "--port", port) //nolint:gosec
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
		syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck
		cmd.Process.Release()              //nolint:errcheck
		return "", fmt.Errorf("proxy daemon did not start: %w (see %s)", err, logFilePath())
	}

	if err := writePIDFile(pid, bound); err != nil {
		syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck
		cmd.Process.Release()              //nolint:errcheck
		return "", fmt.Errorf("error writing proxy PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	cmd.Process.Release() //nolint:errcheck

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
// the body of the hidden `dev proxy serve` subcommand the daemon runs as.
// Once listening it writes the bound port to the port file, which is what
// StartDaemon waits for before writing the PID file.
func Serve(port string) error {
	p := pkgproxy.NewProxy(port, Routes())
	if err := p.Start(); err != nil {
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
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck

	// Poll for exit
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollWait)
		if !isPIDAlive(pid) {
			removeDaemonFiles()
			return nil
		}
	}

	// Force kill
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck
	time.Sleep(stopPollWait)
	removeDaemonFiles()
	return nil
}

func removeDaemonFiles() {
	os.Remove(pidFilePath())
	os.Remove(portFilePath())
}

// StopIfEmpty stops the proxy daemon if there are no active routes.
func StopIfEmpty() {
	routes, err := Routes().ListRoutes()
	if err != nil {
		return
	}
	if len(routes) == 0 {
		logger.Debugf("no active routes, stopping proxy daemon")
		StopDaemon() //nolint:errcheck
	}
}
