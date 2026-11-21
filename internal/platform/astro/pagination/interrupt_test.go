package pagination

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsInterruptAndInterrupted(t *testing.T) {
	assert.True(t, IsInterrupt(context.Canceled))
	assert.True(t, IsInterrupt(fmt.Errorf("listing: %w", context.DeadlineExceeded)))
	assert.False(t, IsInterrupt(errors.New("boom")))
	assert.False(t, IsInterrupt(nil))

	//nolint:staticcheck // a nil context is the case under test
	assert.NoError(t, Interrupted(nil))
	ctx, cancel := context.WithCancel(context.Background())
	assert.NoError(t, Interrupted(ctx))
	cancel()
	assert.ErrorIs(t, Interrupted(ctx), context.Canceled)
}
