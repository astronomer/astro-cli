//go:build !windows

package localstate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockIsExclusive(t *testing.T) {
	setCache(t)
	project := t.TempDir()

	release, err := Lock(project)
	require.NoError(t, err)

	// A second start for the same project cannot take the lock.
	_, err = Lock(project)
	assert.ErrorIs(t, err, ErrLocked)

	// A different project is unaffected.
	other, err := Lock(t.TempDir())
	require.NoError(t, err)
	other()

	// Once released, the project locks again.
	release()
	release2, err := Lock(project)
	require.NoError(t, err)
	release2()
}

// TestLockHammerSerializes proves mutual exclusion under contention: many
// goroutines race for the same project's lock, and no two ever hold it at
// once. Losers retry (Lock is non-blocking), so all eventually pass through.
func TestLockHammerSerializes(t *testing.T) {
	setCache(t)
	project := t.TempDir()

	const workers = 20
	var holders atomic.Int32
	var overlap atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				release, err := Lock(project)
				if err == ErrLocked {
					time.Sleep(time.Millisecond)
					continue
				}
				require.NoError(t, err)
				if holders.Add(1) != 1 {
					overlap.Store(true)
				}
				time.Sleep(time.Millisecond)
				holders.Add(-1)
				release()
				return
			}
		}()
	}
	wg.Wait()
	assert.False(t, overlap.Load(), "two starts held the project lock at the same time")
}
