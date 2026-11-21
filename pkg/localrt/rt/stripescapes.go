package rt

import (
	"regexp"
	"strings"
)

// escapeSeq is a terminal escape sequence: a CSI (colors, cursor controls), an
// OSC (a title, a hyperlink, a clipboard write) ended by BEL or ST, or a DCS,
// SOS, PM or APC string, whose payload runs to its terminator.
// `[ -/]` and `[@-~]` are ECMA-48's intermediate and final byte ranges.
var escapeSeq = regexp.MustCompile("\x1b\\[[0-9;:?]*[ -/]*[@-~]|" + //nolint:gocritic // badRegexp: deliberate code-point ranges, see above
	"\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|" +
	"\x1b[PX^_][^\x1b]*\x1b\\\\")

// StripEscapes removes terminal escape sequences from a log line. Both engines
// use it, so a line prints the same text whichever engine produced it. A line
// with no ESC is returned as is, without running the pattern.
func StripEscapes(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return escapeSeq.ReplaceAllString(s, "")
}
