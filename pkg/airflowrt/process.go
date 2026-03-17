package airflowrt

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	StandalonePIDFile = "airflow.pid"
	StandaloneLogFile = "airflow.log"
	StopPollInterval  = 500 * time.Millisecond
	StopTimeout       = 10 * time.Second
)

// ReadPID reads a PID file and checks if the process is alive.
// Returns the PID and true if the process is running, or 0 and false otherwise.
func ReadPID(pidFilePath string) (int, bool) {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return 0, false
	}

	pid := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid <= 0 {
		return 0, false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	// On Unix, FindProcess always succeeds; use signal 0 to probe.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}

// WritePID writes a PID to the given file path.
func WritePID(pidFilePath string, pid int) error {
	return os.WriteFile(pidFilePath, []byte(fmt.Sprintf("%d", pid)), FilePermissions)
}

// StopProcess stops a process by PID using SIGTERM, then SIGKILL if needed.
// It sends signals to the entire process group (-pid) so child processes are also killed.
// Returns true if the process was found and stopped, false if no process was running.
func StopProcess(pidFilePath string) (bool, error) {
	pid, alive := ReadPID(pidFilePath)
	if pid == 0 {
		return false, nil
	}

	if !alive {
		// Stale PID file — clean up
		os.Remove(pidFilePath)
		return false, nil
	}

	syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck

	// Poll for process exit
	deadline := time.Now().Add(StopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(StopPollInterval)
		if _, stillAlive := ReadPID(pidFilePath); !stillAlive {
			break
		}
	}

	// If still alive, send SIGKILL
	if _, stillAlive := ReadPID(pidFilePath); stillAlive {
		syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
		time.Sleep(StopPollInterval)
	}

	os.Remove(pidFilePath)
	return true, nil
}

// ResolveInEnvPath looks up a binary name in the PATH from the given env slice.
// This is needed because exec.Command uses the parent process's PATH, not cmd.Env.
func ResolveInEnvPath(binary string, env []string) string {
	if filepath.IsAbs(binary) || strings.Contains(binary, string(filepath.Separator)) {
		return binary
	}
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			for _, dir := range filepath.SplitList(e[5:]) {
				candidate := filepath.Join(dir, binary)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}
	return binary // fallback to original
}

// ExecWithEnv runs a command with the given env, dir, and I/O streams.
// It resolves the binary using the env's PATH (not the parent process's PATH)
// so that venv binaries like "airflow" and "pytest" are found correctly.
func ExecWithEnv(dir string, env, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	binary := ResolveInEnvPath(args[0], env)
	cmd := exec.Command(binary, args[1:]...) //nolint:gosec
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// CheckPortAvailable tries to connect to localhost:port. Returns an error if
// something is already listening, so the caller doesn't silently connect to the
// wrong service.
var CheckPortAvailable = func(port string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), time.Second)
	if err != nil {
		return nil // Connection refused / timeout → port is free
	}
	conn.Close()
	return fmt.Errorf("port %s is already in use", port)
}
