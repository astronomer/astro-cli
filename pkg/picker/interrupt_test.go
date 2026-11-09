package picker

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
)

// Canceling the run while the picker waits for a row number ends it at once
// with input.ErrInterrupted, not after a second Ctrl-C or an Enter.
func TestPickEndsOnInterrupt(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { w.Close(); r.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(input.SetInterrupt(ctx))

	got := make(chan error, 1)
	go func() {
		_, err := things().Pick(&bytes.Buffer{}, r)
		got <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-got:
		require.ErrorIs(t, err, input.ErrInterrupted)
	case <-time.After(2 * time.Second):
		t.Fatal("the picker went on waiting after the interrupt")
	}
}
