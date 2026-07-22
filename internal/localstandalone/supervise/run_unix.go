//go:build !windows

package supervise

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const (
	// gracefulStopTimeout is how long the supervisor waits for the child
	// after a shutdown signal or parent death before SIGKILL-ing the
	// process group.
	gracefulStopTimeout = 10 * time.Second
	// logFilePerm is owner-only, matching the rest of the runtime state.
	logFilePerm = 0o600
)

// Run parses supervisor args, launches the child with its output flowing
// into the capped log file, and blocks until the child exits. args is
// everything after the subcommand marker. The caller exits the process with
// the returned error's verdict; Run itself never exits or prints — its own
// diagnostics go into the log file too.
func Run(args []string) error {
	fs := flag.NewFlagSet(Subcommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parentPID := fs.Int(parentPIDFlagName, 0, "PID to watch; the supervisor kills the child when this PID exits (0 watches nothing)")
	logPath := fs.String(logFileFlagName, "", "file the child's output and the supervisor's diagnostics are written to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *logPath == "" || len(rest) == 0 {
		return fmt.Errorf("usage: %s %s <path> [%s <pid>] -- <cmd> [args...]", Subcommand, LogFileFlag, ParentPIDFlag)
	}

	// O_RDWR, not O_WRONLY: the capped writer reads the file's tail back
	// (ReadAt) when it truncates, and ReadAt on a write-only fd fails with
	// EBADF — which would silently defeat the cap and let the log grow
	// without bound.
	logFile, err := os.OpenFile(*logPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, logFilePerm) //nolint:gosec // the engine derives this path from the project's state dir
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()
	cw := newCappedWriter(logFile)
	lg := log.New(cw, "supervise: ", log.LstdFlags)

	// Trap SIGTERM/SIGINT so signaling our own pgid (below) doesn't kill us
	// before we finish cleaning up the Airflow subprocesses. Also covers the
	// normal stop path, where the engine signals the pgid directly — we stay
	// alive until Airflow is fully stopped, then return.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	cmd := exec.Command(rest[0], rest[1:]...) //nolint:gosec // the engine builds this command line from its own venv layout
	cmd.Stdout = cw
	cmd.Stderr = cw
	cmd.Stdin = nil
	// No Setpgid here: the child inherits the supervisor's pgid, so a
	// single kill(-pgid, ...) from the stop path reaches everyone.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", rest[0], err)
	}

	// A nil channel blocks forever, so without a watched parent the select
	// below simply never takes that arm.
	var parentDead chan struct{}
	if *parentPID > 0 {
		parentDead = make(chan struct{})
		go func() {
			waitForProcessExit(*parentPID)
			close(parentDead)
		}()
	}

	childExited := make(chan error, 1)
	go func() { childExited <- cmd.Wait() }()

	lg.Printf("watching parent=%d, child=%d", *parentPID, cmd.Process.Pid)

	select {
	case err := <-childExited:
		// Airflow exited on its own; report how.
		lg.Printf("child exited on its own: %v", err)
		return err
	case sig := <-sigCh:
		// Normal shutdown: the stop path signaled our pgid. Airflow already
		// received it too; wait for it to finish, then escalate.
		lg.Printf("received %v, waiting for child to exit", sig)
		return waitOrKill(childExited, lg)
	case <-parentDead:
		// Session end: the watched parent died without running its stop
		// path. Signal the whole pgid (so Airflow's scheduler/api-server/
		// triggerer get SIGTERM too, not just the master) and wait for exit.
		lg.Printf("parent died, SIGTERM-ing own pgid")
		signalOwnPgroup(syscall.SIGTERM)
		return waitOrKill(childExited, lg)
	}
}

// waitOrKill waits for the Airflow master to exit. If it doesn't within the
// graceful window, SIGKILLs the whole pgid (catches stuck subprocesses).
func waitOrKill(childExited <-chan error, lg *log.Logger) error {
	select {
	case err := <-childExited:
		lg.Printf("child exited, supervisor done")
		return shutdownVerdict(err)
	case <-time.After(gracefulStopTimeout):
		lg.Printf("child did not exit within %v, SIGKILL-ing own pgid", gracefulStopTimeout)
		signalOwnPgroup(syscall.SIGKILL)
		// SIGKILL to our own pgid kills us too, but be defensive.
		return shutdownVerdict(<-childExited)
	}
}

// shutdownVerdict maps the child's exit during a deliberate shutdown to
// nil: dying from the stop signal is the intended outcome, not a failure.
func shutdownVerdict(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return err
}
