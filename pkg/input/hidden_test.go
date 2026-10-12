package input

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paste wraps s the way a terminal with bracketed paste turned on sends it.
func paste(s string) string { return "\x1b[200~" + s + "\x1b[201~" }

func TestReadHidden(t *testing.T) {
	long := `{"conn_type":"google_cloud_platform","extra":{"keyfile_dict":"` + strings.Repeat("x", 4000) + `"}}`
	pem := "-----BEGIN PRIVATE KEY-----\nMIIEvQ\nIBADAN\n-----END PRIVATE KEY-----"
	cases := []struct {
		name, in, want string
	}{
		{"enter ends a typed value", "s3cret\rignored", "s3cret"},
		{"a typed newline ends it too", "s3cret\n", "s3cret"},
		{"an unbalanced brace is just a character", "{p@ss\r", "{p@ss"},
		{"a long line is not truncated", long + "\r", long},
		{"a paste keeps its newlines until enter", paste("{\r  \"conn_type\": \"http\"\r}") + "\r", "{\n  \"conn_type\": \"http\"\n}"},
		{"a paste ending in a newline ends the value", paste(pem+"\n") + "left for the shell", pem},
		{"a pasted crlf is one line break", paste("a\r\nb\r\n\r\nc") + "\r", "a\nb\n\nc"},
		{"typing continues after a paste", paste("abc") + "def\r", "abcdef"},
		{"ctrl-d ends the value", "abc\x04rest", "abc"},
		{"backspace removes a whole rune", "aé\x7f\x08b\r", "b"},
		{"ctrl-u clears the line", "wrong\x15right\r", "right"},
		{"csi arrow keys are dropped", "ab\x1b[Dc\r", "abc"},
		{"ss3 keys are dropped", "a\x1bOPb\x1bOAc\r", "abc"},
		{"a bare esc drops only itself", "\x1bs3cret\r", "s3cret"},
		{"other control bytes are dropped", "a\x01b\tc\r", "ab\tc"},
		{"eof returns what was typed", "partial", "partial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readHidden(strings.NewReader(tc.in))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Nothing past the end of the value is taken off the terminal: what follows
// Enter is still there for whoever reads next.
func TestReadHiddenLeavesTheRestUnread(t *testing.T) {
	r := strings.NewReader("value\rnext")
	got, err := readHidden(r)
	require.NoError(t, err)
	assert.Equal(t, "value", got)
	rest, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "next", string(rest))
}

func TestReadHiddenCtrlC(t *testing.T) {
	_, err := readHidden(strings.NewReader("half\x03rest\r"))
	require.ErrorIs(t, err, ErrInterrupted)
	_, err = readHidden(strings.NewReader(paste("half") + "\x03"))
	require.ErrorIs(t, err, ErrInterrupted, "Ctrl-C after a paste")
}

func TestReadHiddenEmptyEOF(t *testing.T) {
	for _, in := range []string{"", "\x04"} {
		_, err := readHidden(strings.NewReader(in))
		require.ErrorIs(t, err, io.EOF, "input %q", in)
	}
}
