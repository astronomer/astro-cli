package airflowrt

import (
	"fmt"
	"net"
	"os"
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

// CheckPortAvailable tries to connect to localhost:port. Returns an error if
// something is already listening, so the caller doesn't silently connect to the
// wrong service.
var CheckPortAvailable = func(port string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), time.Second)
	if err != nil {
		return nil // Connection refused / timeout → port is free
	}
	conn.Close() //nolint:errcheck // the dial answered the question; the close is tidiness
	return fmt.Errorf("port %s is already in use", port)
}
