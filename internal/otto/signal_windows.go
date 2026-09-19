//go:build windows

package otto

import (
	"os"
	"os/exec"
	"os/signal"
)

func forwardSignals(cmd *exec.Cmd) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for sig := range sigCh {
			if cmd.Process != nil {
				// A child that has exited between the nil check and here cannot
				// be signaled, and there is nobody to tell: this goroutine
				// exists to forward, not to report.
				_ = cmd.Process.Signal(sig) //nolint:errcheck // deliberate, for the reason above
			}
		}
	}()
}
