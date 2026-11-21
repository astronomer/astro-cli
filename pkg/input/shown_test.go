package input

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A question shows the names it quotes as text: the CLI's own bold stays, and
// an escape a name carries (a window title, a screen clear) is not passed to
// the terminal. Styling a name leaves on is reset before the answer is typed.
func TestConfirmationShowsTheQuestionAsText(t *testing.T) {
	var out bytes.Buffer
	err := (&Confirmation{
		Question: "Delete \x1b[1mprod\x1b]0;title\x07\x1b[2J\x1b[7m\x1b[0m?",
		In:       strings.NewReader("y\n"),
		Out:      &out,
	}).Ask()
	require.NoError(t, err)
	got := out.String()
	assert.Contains(t, got, "\x1b[1mprod", "the CLI's own bold is kept")
	assert.NotContains(t, got, "\x1b]0;")
	assert.NotContains(t, got, "\x1b[2J")
}
