//go:build windows

package agent

import (
	"os"
	"os/exec"
	"os/signal"
)

// execAgent runs the agent binary as a child process on Windows.
// Windows doesn't support syscall.Exec (process replacement), so we
// run it as a subprocess and forward stdio + signals.
func execAgent(binPath string, args []string) error {
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Forward interrupt signals to the child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		for range sigCh {
			// On Windows, os.Interrupt is sent to the whole process group,
			// so the child already receives it. This goroutine just prevents
			// the parent from exiting early.
		}
	}()

	return cmd.Wait()
}
