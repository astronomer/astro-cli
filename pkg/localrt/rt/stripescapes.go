package rt

import (
	"regexp"
	"strings"
	"unicode"
)

// escapeSeq is a terminal escape sequence, in its 7-bit form (ESC first) or its
// C1 form (one control character, U+009B CSI, U+009D OSC and so on, which some
// terminals read as the escape it stands for):
//   - a CSI (colors, cursor controls);
//   - an OSC (a title, a hyperlink, a clipboard write) ended by BEL or ST;
//   - a DCS, SOS, PM or APC string, whose payload runs to its terminator;
//   - one that never finished, which takes the rest of the line with it, so
//     its parameters or payload do not print as text;
//   - a two-byte escape such as ESC c (reset) or ESC 7 (save cursor).
//
// `[ -/]`, `[0-~]` and `[@-~]` are ECMA-48's intermediate and final byte
// ranges. The alternatives are tried in order, so a finished sequence wins
// over the unfinished one that would also match its start.
var escapeSeq = regexp.MustCompile("(?s)" + //nolint:gocritic // badRegexp: deliberate code-point ranges, see above
	"\x1b\\[[0-9;:?]*[ -/]*[@-~]|\u009b[0-9;:?]*[ -/]*[@-~]|" +
	"(?:\x1b\\]|\u009d)[^\x07\x1b\u009c]*(?:\x07|\x1b\\\\|\u009c)|" +
	"(?:\x1b[PX^_]|[\u0090\u0098\u009e\u009f])[^\x1b\u009c]*(?:\x1b\\\\|\u009c)|" +
	"(?:\x1b[\\[\\]PX^_]|[\u009b\u009d\u0090\u0098\u009e\u009f]).*$|" +
	"\x1b[ -/]*[0-~]")

// StripEscapes removes what a terminal would act on rather than show from a
// log line: escape sequences, any other control character (a lone ESC, a bell,
// a backspace, a carriage return, which would let a later part of the line
// overwrite an earlier one), the C1 controls, DEL, and the invisible
// characters that reorder or hide text (the bidirectional controls and the
// zero-width space among them). A tab stays, and so does the zero-width joiner
// that emoji sequences are built from. Both engines use it, so a line prints
// the same text whichever engine produced it. The line comes without its line
// break, so every carriage return in it is stray.
//
// A line of printable ASCII, the usual log line, is returned as is after one
// pass over its bytes, without running the pattern or allocating.
func StripEscapes(s string) string {
	if isPlainASCII(s) {
		return s
	}
	if hasEscapeIntroducer(s) {
		s = escapeSeq.ReplaceAllString(s, "")
	}
	if strings.IndexFunc(s, dropRune) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if dropRune(r) {
			return -1
		}
		return r
	}, s)
}

// isPlainASCII reports whether s has nothing StripEscapes could change: no
// control character but a tab, no DEL and no byte outside ASCII.
func isPlainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i]; (b < 0x20 && b != '\t') || b >= 0x7f {
			return false
		}
	}
	return true
}

// hasEscapeIntroducer reports whether s has an ESC or a C1 control (U+0080 to
// U+009F, which UTF-8 writes as 0xC2 and a byte below 0xA0), so a line with
// only accented letters skips the pattern.
func hasEscapeIntroducer(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i]; b == 0x1b || (b == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] < 0xa0) {
			return true
		}
	}
	return false
}

// dropRune is a character StripEscapes removes once the escape sequences are
// gone: a control character but a tab, or one of misleadingRune.
func dropRune(r rune) bool {
	return (r != '\t' && unicode.IsControl(r)) || misleadingRune(r)
}

// misleadingRune is a character with no glyph of its own that reorders the
// text around it or hides in it: the bidirectional controls, the zero-width
// space, the byte order mark, the word joiner and the invisible math
// operators, the deprecated formatting controls and the interlinear annotation
// marks. The zero-width joiner and non-joiner, the variation selectors and the
// tag characters stay: emoji sequences and Persian and Indic words use them.
// It is pkg/input's rule of the same name; this module cannot import it.
func misleadingRune(r rune) bool {
	switch {
	case r >= '\u202a' && r <= '\u202e', // embeddings and overrides
		r >= '\u2066' && r <= '\u2069',              // isolates
		r == '\u200e', r == '\u200f', r == '\u061c', // marks
		r == '\u200b', r == '\ufeff', // zero-width space, byte order mark
		r >= '\u2060' && r <= '\u2064', // word joiner, invisible operators
		r >= '\u206a' && r <= '\u206f', // deprecated formatting controls
		r >= '\ufff9' && r <= '\ufffb': // interlinear annotation
		return true
	}
	return false
}
