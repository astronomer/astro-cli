package cliout

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter refuses every write.
type failingWriter struct{ writes int }

var errRefused = errors.New("refused")

func (f *failingWriter) Write([]byte) (int, error) {
	f.writes++
	return 0, errRefused
}

// A renderer checks no write of its own, so the failure has to come back from
// WriteText, however much the renderer wrote after it.
func TestWriteTextReportsAFailedWriteOnce(t *testing.T) {
	w := &failingWriter{}
	err := WriteText(w, func(b *bufio.Writer) {
		for range 10_000 { // well past the buffer, so a write reaches w mid-render
			fmt.Fprintln(b, "a line")
		}
	})
	require.ErrorIs(t, err, errRefused)
	assert.Equal(t, 1, w.writes, "after the first failure the buffer writes nothing more")

	var out bytes.Buffer
	require.NoError(t, Text(func(b *bufio.Writer) { fmt.Fprint(b, "hello") })(&out))
	assert.Equal(t, "hello", out.String(), "Text flushes what was written")
}
