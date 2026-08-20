//go:build !windows

package supervise

import (
	"os"
	"syscall"
)

// signalOwnPgroup sends sig to every process in our process group. Used to
// propagate shutdown to Airflow's subprocesses, which inherit our pgid.
func signalOwnPgroup(sig syscall.Signal) {
	_ = syscall.Kill(-os.Getpid(), sig) //nolint:errcheck // best-effort shutdown signal to our process group
}
