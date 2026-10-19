//go:build windows

package localdocker

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

// taskkillTimeout bounds the tree kill, which runs with the start already
// failing.
const taskkillTimeout = 10 * time.Second

// killTreeOnCancel makes cmd's cancel kill its whole process tree, so the
// children it spawned go with it. Windows has no process groups to signal;
// taskkill /T walks the tree from the PID, and ships with every Windows.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), taskkillTimeout)
		defer cancel()
		//nolint:errcheck,gosec // best effort: the Kill below still ends the CLI itself
		exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		return cmd.Process.Kill()
	}
}
