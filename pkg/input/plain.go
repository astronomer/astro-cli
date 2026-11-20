package input

import (
	"regexp"
	"strings"
	"unicode"
)

// ansiEscape is a terminal escape sequence: a CSI (the styles ansi.Bold wraps
// a name in, cursor controls with private parameters) or an OSC (a hyperlink,
// a title), as pkg/localrt strips from logs, or a device control, privacy or
// application string, whose payload would otherwise print as text.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;:?]*[\\x20-\\x2f]*[\\x40-\\x7e]|" +
	"\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|" +
	// A DCS, SOS, PM or APC string runs to its terminator, payload included.
	"\x1b[PX^_][^\x1b]*\x1b\\\\")

// Plain is text as a message or a log line shows it: without the escapes it
// was styled with, without any other control character (a lone ESC, a bell,
// a backspace) or invisible character that makes text read as something it is
// not (misleadingRune), and on one line. Other format characters stay: the
// zero-width joiner and non-joiner and the variation selectors are part of
// emoji sequences and of Persian and Indic words. It is how a prompt is quoted
// in a refusal, and how astro local's [y/N] reads and echoes an answer from a
// pipe.
func Plain(s string) string {
	return strings.Join(strings.Fields(strip(s, true)), " ")
}

// Clean is s without escapes, control characters (a newline or a tab
// included) or misleadingRune characters, and otherwise as it was: not
// trimmed, and with every space kept, so two names that differ in their
// spacing still differ. It is for text that names something on this machine
// (a project's path in a question) and must not be altered, only made safe to
// print.
func Clean(s string) string { return strip(s, false) }

// Cell is text as one table cell shows it: a line break, a tab, a vertical
// tab or a form feed becomes a space (each of them would end the cell or the
// row), a carriage return goes, and then escapes, control characters and
// misleadingRune characters go, as Clean removes them. Unlike Plain it keeps
// the text's own spacing: a name with two spaces in it still shows two. It is
// for a value from somewhere else (an API's name or description) in a table.
func Cell(s string) string {
	return strip(cellSpace.Replace(s), false)
}

// cellSpace is the white space Cell turns into a space before it strips.
var cellSpace = strings.NewReplacer("\r\n", " ", "\n", " ", "\t", " ", "\v", " ", "\f", " ", "\r", "")

// Lines is text as a multi-line value shows it: without escapes, control
// characters or misleadingRune characters, but with its line breaks and tabs,
// so a pretty-printed JSON value still reads as one. A CRLF line break becomes
// a plain newline, and a carriage return on its own, which would let a later
// part of the text overwrite an earlier one on screen, is dropped. It is for
// text from somewhere else (an Airflow API response) printed as it is.
func Lines(s string) string {
	s = ansiEscape.ReplaceAllString(strings.ReplaceAll(s, "\r\n", "\n"), "")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || misleadingRune(r) {
			return -1
		}
		return r
	}, s)
}

// strip removes escapes, control characters and misleadingRune characters from
// s. keepSpace keeps the white space (a newline or a tab) for the caller to
// collapse.
func strip(s string, keepSpace bool) string {
	s = ansiEscape.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && !(keepSpace && unicode.IsSpace(r))) || misleadingRune(r) {
			return -1
		}
		return r
	}, s)
}

// misleadingRune is a character with no glyph of its own that reorders the
// text around it or hides in it: the bidirectional controls (an override can
// show a different run ID from the one acted on), the zero-width space, the
// byte order mark, the word joiner and the invisible math operators, the
// deprecated formatting controls and the interlinear annotation marks. The tag
// characters stay, because flag emoji such as England's are written with them.
func misleadingRune(r rune) bool {
	switch {
	case r >= '\u202A' && r <= '\u202E', // embeddings and overrides
		r >= '\u2066' && r <= '\u2069',              // isolates
		r == '\u200E', r == '\u200F', r == '\u061C', // marks
		r == '\u200B', r == '\uFEFF', // zero-width space, byte order mark
		r >= '\u2060' && r <= '\u2064', // word joiner, invisible operators
		r >= '\u206A' && r <= '\u206F', // deprecated formatting controls
		r >= '\uFFF9' && r <= '\uFFFB': // interlinear annotation
		return true
	}
	return false
}

// Quote is prompt in double quotes, as a message names a question: its Plain
// text with a literal double quote escaped and nothing else. Not %q, which
// would also double every backslash, so a Windows path in the question
// (C:\Users\me\proj) would read C:\\Users\\me\\proj, and again in json.
func Quote(prompt string) string { return quotePlain(Plain(prompt)) }

// quotePlain is Quote for text that is already Plain.
func quotePlain(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
