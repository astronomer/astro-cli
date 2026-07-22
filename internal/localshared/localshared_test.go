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
}
