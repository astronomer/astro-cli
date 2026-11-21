package picker

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A row's cells are shown as text on one line: a name's escapes are not
// passed to the terminal and its line break does not split the row, so the
// table still lines up and the number still names the row it is beside.
func TestPickShowsCellsAsText(t *testing.T) {
	l := &List{Title: "Pick \x1b]0;title\x07one", Header: []string{"NAME", "DESCRIPTION"}, Invalid: errInvalidThing}
	l.AddRow(false, "prod\x1b]0;title\x07", "line one\nline two\x1b[2J")
	l.AddRow(true, "dev", "ok")
	var out bytes.Buffer
	i, err := l.Pick(&out, strings.NewReader("2\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, i)
	got := out.String()
	assert.NotContains(t, got, "\x1b]0;")
	assert.NotContains(t, got, "\x1b[2J")
	assert.Contains(t, got, "Pick one")
	var row string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "1 ") {
			row = line
		}
	}
	assert.Contains(t, row, "prod")
	assert.Contains(t, row, "line one line two", "the description stays on its row")
	assert.Contains(t, got, "\x1b[", "the current row's highlight is kept")
}
