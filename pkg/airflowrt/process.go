package airflowrt

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	StandalonePIDFile     = "airflow.pid"
	StandaloneLogFile     = "airflow.log"
	StandaloneVersionFile = "airflow_version"
	StopPollInterval      = 500 * time.Millisecond
	StopTimeout           = 10 * time.Second
)

// stopPollInterval and stopTimeout mirror the exported constants; tests
// shorten them to exercise the stop path without real processes.
// killTimeout bounds the wait for a group to go after SIGKILL.
var (
	stopPollInterval = StopPollInterval
	stopTimeout      = StopTimeout
	killTimeout      = 5 * time.Second
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

	if !isProcessAlive(pid) {
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
// It removes the PID file only once the whole group is gone: a group that
// outlives SIGKILL is an error, and its PID file is kept so a later stop can
// still reach it.
func StopProcess(pidFilePath string) (bool, error) {
	pid, _ := ReadPID(pidFilePath)
	if pid == 0 {
		return false, nil
	}

	// The process was started with Setpgid, so its group, not its PID, says
	// whether anything is left to stop. The master can be gone while its
	// scheduler/api-server/triggerer run on in the group (it crashed, was
	// OOM-killed, or took a SIGTERM of its own), and a PID is not reused
	// while a group with that ID exists, so a live group is still ours. A
	// live PID with no group is not: the PID file outlived its process and
	// the PID went to an unrelated one, which must not be signaled.
	if !isProcessGroupAlive(pid) {
		// Stale PID file — clean up
		os.Remove(pidFilePath)
		return false, nil
	}

	terminateProcess(pid)

	// Poll the process group — not just the master PID. airflow standalone
	// spawns scheduler/api-server/triggerer as subprocesses that inherit the
	// group; the master often exits on SIGTERM well before its children finish
	// graceful shutdown. Watching only the master would return early here and
	// leave those subprocesses running.
	if !waitGroupGone(pid, stopTimeout) {
		// If any group member is still alive, force kill
		killProcess(pid)
		if !waitGroupGone(pid, killTimeout) {
			return true, fmt.Errorf("process group %d is still running after SIGKILL; kept %s so a later stop can reach it", pid, pidFilePath)
		}
	}

	os.Remove(pidFilePath)
	return true, nil
}

// waitGroupGone polls until no process in pid's group is alive, or timeout
// passes, and reports whether the group is gone.
func waitGroupGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollInterval)
		if !isProcessGroupAlive(pid) {
			return true
		}
	}
	return !isProcessGroupAlive(pid)
}

// ProcessGroupAlive reports whether any process in pid's group is still
// running. The process must have been started with Setpgid so pid is the
// group leader. On Windows, where Unix process groups don't apply, it checks
// the single PID.
func ProcessGroupAlive(pid int) bool {
	return isProcessGroupAlive(pid)
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
