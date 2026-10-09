//go:build !windows

package proxy

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// startDetached starts the command build returns in a process group of its
// own, so the daemon outlives the shell or app that started it and does not
// receive the signals sent to that group.
func startDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := build()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return cmd, cmd.Start()
}

// requestStop asks the daemon at pid to stop by sending it SIGTERM. It always
// reports true: whether the signal landed is what the wait that follows finds
// out.
var requestStop = func(pid int) bool {
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // the wait checks the outcome
	return true
}

// killProcess ends pid without asking.
var killProcess = func(pid int) {
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // nothing left to try
}

// stopRequests returns a channel that is closed when the daemon is asked to
// stop: SIGTERM from requestStop, or SIGINT from a terminal running it in the
// foreground. The func it returns stops listening.
func stopRequests() (<-chan struct{}, func(), error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	stop := make(chan struct{})
	go func() {
		<-sigCh
		close(stop)
	}()
	return stop, func() { signal.Stop(sigCh) }, nil
}

// processLooksLikeProxy reports whether pid's command line looks like the
// daemon, which runs with ServeArgs. It's the fallback for when the recorded
// port does not answer the signature probe in time.
var processLooksLikeProxy = func(d *Daemon, pid int) bool {
	if len(d.ServeArgs) == 0 {
		return false
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // a pid, formatted as an integer
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strings.Join(d.ServeArgs, " "))
}
