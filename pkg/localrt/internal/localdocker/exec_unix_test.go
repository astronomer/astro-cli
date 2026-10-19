//go:build !windows

package localdocker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// A deadline kills the children a command spawned, not only the command.
//
// `docker compose` runs the compose plugin as a child of the docker CLI, and
// the default cancel killed the CLI alone: the plugin kept driving the up
// while the rollback took the project down around it.
func TestACanceledCommandTakesItsChildrenWithIt(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := execCommander{}.Run(ctx, nil, rt.Stdio{}, "sh", "-c", `sleep 30 & echo $! > "$0"; wait`, pidFile)
	require.Error(t, err)

	raw, rerr := os.ReadFile(pidFile)
	require.NoError(t, rerr)
	pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, perr)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if kerr := syscall.Kill(pid, 0); errors.Is(kerr, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the child %d outlived its canceled parent", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
