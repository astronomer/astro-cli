package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/pkg/logger"
)

const (
	pidFileName  = "proxy.pid"
	logFileName  = "proxy.log"
	stopTimeout  = 5 * time.Second
	stopPollWait = 500 * time.Millisecond
)

// pidFilePath returns the path to ~/.astro/proxy/proxy.pid.
func pidFilePath() string {
	return filepath.Join(proxyDirPath(), pidFileName)
}

// logFilePath returns the path to ~/.astro/proxy/proxy.log.
func logFilePath() string {
	return filepath.Join(proxyDirPath(), logFileName)
}

// IsRunning checks if the proxy daemon is running by reading the PID file
// and verifying the process is alive.
func IsRunning() (int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}

	if !isPIDAlive(pid) {
		return pid, false
	}
	return pid, true
}

// EnsureRunning starts the proxy daemon if it's not already running.
// Returns the port the proxy is listening on.
func EnsureRunning(port string) (string, error) {
	if port == "" {
		port = DefaultPort
	}

	if _, alive := IsRunning(); alive {
		logger.Debugf("proxy daemon already running")
		return port, nil
	}

	// Clean up stale PID file
	os.Remove(pidFilePath())

	return port, StartDaemon(port)
}

// StartDaemon starts the proxy as a background process.
// It re-executes the current CLI binary with a hidden subcommand.
var StartDaemon = func(port string) error {
	if err := os.MkdirAll(proxyDirPath(), dirPermRWX); err != nil {
		return fmt.Errorf("error creating proxy directory: %w", err)
	}

	logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermRW)
	if err != nil {
		return fmt.Errorf("error opening proxy log file: %w", err)
	}

	// Find the current CLI binary
	exe, err := os.Executable()
	if err != nil {
		logFile.Close()
		return fmt.Errorf("error finding CLI executable: %w", err)
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
		logFile.Close()
		return fmt.Errorf("error starting proxy daemon: %w", err)
	}

	// Write PID file
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidFilePath(), []byte(strconv.Itoa(pid)), filePermRW); err != nil {
		syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck
		logFile.Close()
		return fmt.Errorf("error writing proxy PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	cmd.Process.Release() //nolint:errcheck
	logFile.Close()

	logger.Debugf("proxy daemon started (PID %d) on port %s", pid, port)
	return nil
}

// StopDaemon stops the proxy daemon by sending SIGTERM, then SIGKILL if needed.
func StopDaemon() error {
	pid, alive := IsRunning()
	if !alive {
		if pid > 0 {
			os.Remove(pidFilePath())
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
			os.Remove(pidFilePath())
			return nil
		}
	}

	// Force kill
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck
	time.Sleep(stopPollWait)
	os.Remove(pidFilePath())
	return nil
}

// StopIfEmpty stops the proxy daemon if there are no active routes.
func StopIfEmpty() {
	routes, err := ListRoutes()
	if err != nil {
		return
	}
	if len(routes) == 0 {
		logger.Debugf("no active routes, stopping proxy daemon")
		StopDaemon() //nolint:errcheck
	}
}
