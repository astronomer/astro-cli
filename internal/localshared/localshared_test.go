package localshared

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineWriterBuffersPartialWrites(t *testing.T) {
	t.Parallel()
	var got []string
	w := &LineWriter{Emit: func(s string) { got = append(got, s) }}

	for _, chunk := range []string{"first li", "ne\r\nsecond", " line\npart", "ial"} {
		_, err := w.Write([]byte(chunk))
		assert.NoError(t, err)
	}
	assert.Equal(t, []string{"first line", "second line"}, got)
	w.Flush()
	assert.Equal(t, []string{"first line", "second line", "partial"}, got)
}

func TestChoosePort(t *testing.T) {
	t.Parallel()
	free := func(string) bool { return true }
	noAlloc := func() (string, error) { return "", errors.New("allocation not expected") }

	// The requested port wins when free.
	got, err := ChoosePort(8081, 8080, free, noAlloc)
	require.NoError(t, err)
	assert.Equal(t, 8081, got)

	// No request: the mode default when free.
	got, err = ChoosePort(0, 8080, free, noAlloc)
	require.NoError(t, err)
	assert.Equal(t, 8080, got)

	// A busy requested port allocates instead of colliding with the default.
	busy := func(string) bool { return false }
	alloc := func() (string, error) { return "10123", nil }
	got, err = ChoosePort(8081, 8080, busy, alloc)
	require.NoError(t, err)
	assert.Equal(t, 10123, got)

	// An excluded default falls through to allocation, so a sibling port can't
	// be reused.
	got, err = ChoosePort(0, 8080, free, alloc, 8080)
	require.NoError(t, err)
	assert.Equal(t, 10123, got)

	// Allocation retries past an excluded pool port and returns the next one.
	seq := []string{"10123", "10124"}
	i := 0
	stepAlloc := func() (string, error) {
		s := seq[i]
		i++
		return s, nil
	}
	got, err = ChoosePort(0, 8080, busy, stepAlloc, 10123)
	require.NoError(t, err)
	assert.Equal(t, 10124, got)

	// A degenerate allocator that only ever returns the excluded port gives up
	// rather than looping forever.
	stuck := func() (string, error) { return "10123", nil }
	_, err = ChoosePort(0, 8080, busy, stuck, 10123)
	require.Error(t, err)
}
