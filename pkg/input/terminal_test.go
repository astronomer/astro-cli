package input

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Terminal keeps the styling the CLI uses and drops anything else a terminal
// would act on. Every escape is written as one here, so the source shows it.
func TestTerminal(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain text is unchanged", "my deployment", "my deployment"},
		{"line breaks and tabs stay", "a\n\tb", "a\n\tb"},
		{"a CRLF becomes a newline", "a\r\nb", "a\nb"},
		{"SGR styling stays", "\x1b[1mbold\x1b[0m \x1b[1;32mgreen\x1b[0m", "\x1b[1mbold\x1b[0m \x1b[1;32mgreen\x1b[0m"},
		{"a clipboard write goes", "x\x1b]52;c;aGk=\x07y", "xy"},
		{"a title, ended by ST, goes", "x\x1b]0;pwned\x1b\\y", "xy"},
		{"a screen clear and a cursor move go", "\x1b[2J\x1b[Hfake", "fake"},
		{"a device control string goes", "x\x1bPpayload\x1b\\y", "xy"},
		{"a lone ESC, a bell and a backspace go", "a\x1bb\x07c\x08d", "abcd"},
		{"a lone carriage return goes", "safe\rspoofed", "safespoofed"},
		{"a C1 CSI goes", "a\u009b2Jb", "a2Jb"},
		{"a right-to-left override goes", "run \u202Egnp.exe", "run gnp.exe"},
		{"text in another script is unchanged", "café 日本", "café 日本"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, Terminal(tc.in)) })
	}
}

// writeAll writes each piece in its own Write, then flushes, and returns what
// reached the underlying writer.
func writeAll(t *testing.T, pieces ...string) string {
	t.Helper()
	var out bytes.Buffer
	tw := &TerminalWriter{W: &out}
	for _, p := range pieces {
		n, err := tw.Write([]byte(p))
		require.NoError(t, err)
		require.Equal(t, len(p), n)
	}
	require.NoError(t, tw.Flush())
	return out.String()
}

// A sequence or a character one write splits from the next is held for the
// rest, so it is judged whole: a split clipboard write still goes, a split
// color still stays, a split character is not mangled.
func TestTerminalWriterJoinsWhatWritesSplit(t *testing.T) {
	assert.Equal(t, "ab", writeAll(t, "a\x1b]52;c;", "aGk=\x07b"), "a clipboard write split across writes")
	assert.Equal(t, "ab", writeAll(t, "a\x1b", "]52;c;aGk=\x07b"), "split right after the ESC")
	assert.Equal(t, "\x1b[31mred\x1b[0m", writeAll(t, "\x1b[3", "1mred\x1b[", "0m"), "a color split across writes")
	assert.Equal(t, "café", writeAll(t, "caf\xc3", "\xa9"), "a character split across writes")
	assert.Equal(t, "fake", writeAll(t, "\x1b[2", "J", "fake"), "a screen clear split across writes")
}

// Nothing is held for good: an escape no write finishes is dropped at Flush,
// and one held past the cap is released with its ESC dropped, so the rest
// prints as text rather than as a command.
func TestTerminalWriterHoldsNothingForGood(t *testing.T) {
	assert.Equal(t, "ok", writeAll(t, "ok\x1b]52;c;never-ended"), "Flush drops an unfinished sequence")

	var out bytes.Buffer
	tw := &TerminalWriter{W: &out}
	_, err := tw.Write([]byte("\x1b]0;" + strings.Repeat("x", maxHeldEscape+1)))
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "\x1b", "a sequence held past the cap is released without its ESC")
	assert.Contains(t, out.String(), "]0;xxx")
}
