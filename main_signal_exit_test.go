//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The exit status names the signal that ended the run, as a shell would:
// 128+n. SIGHUP (a closed terminal) cancels like the others, so the rollback
// on the way out runs for it too.
func TestSignalsCancelAndSetTheExitStatus(t *testing.T) {
	for _, tc := range []struct {
		sig  syscall.Signal
		want int
	}{
		{syscall.SIGINT, 130},
		{syscall.SIGTERM, 143},
		{syscall.SIGHUP, 129},
	} {
		t.Run(tc.sig.String(), func(t *testing.T) {
			ctx := signalContext()
			if err := syscall.Kill(os.Getpid(), tc.sig); err != nil {
				t.Fatalf("raise %v: %v", tc.sig, err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatalf("%v did not cancel the context", tc.sig)
			}
			if got := cliout.ExitCode(ctx, errors.New("boom")); got != tc.want {
				t.Errorf("exit status after %v = %d, want %d", tc.sig, got, tc.want)
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
			}
		})
	}
}
