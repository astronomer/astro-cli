//go:build !windows

package localstandalone

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// A canceled foreground command is asked to stop with SIGTERM, so it can clean
// up, rather than killed outright.
func TestExecCommanderCancelSendsSIGTERM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	ready, term := filepath.Join(dir, "ready"), filepath.Join(dir, "term")
	script := `trap 'touch "$2"; exit 0' TERM; touch "$1"; while :; do sleep 0.05; done`
	done := make(chan error, 1)
	go func() {
		done <- execCommander{}.Run(ctx, dir, os.Environ(), rt.Stdio{}, "sh", "-c", script, "sh", ready, term)
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(ready); return err == nil }, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the command did not stop after cancel")
	}
	assert.FileExists(t, term, "the command was terminated, not killed")
}

// A command that exits 0 while a background child still holds its output is a
// success, not an ErrWaitDelay failure, on a run nobody canceled.
func TestExecCommanderIgnoresWaitDelayWhenNotCanceled(t *testing.T) {
	var out strings.Builder
	err := execCommander{}.Run(context.Background(), t.TempDir(), os.Environ(), rt.Stdio{Out: &out},
		"sh", "-c", "sleep 8 & echo done")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "done")
}
