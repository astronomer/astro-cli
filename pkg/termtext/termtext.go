// Package termtext is what text from somewhere else (an API's names, a
// Deployment's or a local Airflow's log lines) has removed before a terminal
// is given it: the escape sequences and control characters a terminal acts on
// rather than shows, and the invisible characters that reorder or hide text.
// It is one set of rules for every caller, the CLI's pkg/input and the local
// runtime's log lines alike, so the two cannot drift apart. It has no
// dependencies outside its tests.
package termtext

import (
	"regexp"
	"strings"
	"unicode"
)

// Escape is one whole terminal escape sequence: a CSI (the styles a name is
// wrapped in, cursor controls with private parameters), an OSC (a hyperlink, a
// title, a clipboard write) ended by BEL or ST, or a device control, privacy
// or application string, whose payload runs to its terminator. It matches
// only finished sequences, so a writer that streams text can tell a sequence
// it has whole from one a later write finishes (pkg/input's TerminalWriter).
var Escape = regexp.MustCompile(escapePattern)

// escapePattern is Escape's source. `[\x20-\x2f]` and `[\x40-\x7e]` are
// ECMA-48's intermediate and final byte ranges.
const escapePattern = "\x1b\\[[0-9;:?]*[\\x20-\\x2f]*[\\x40-\\x7e]|" +
	"\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|" +
	// A DCS, SOS, PM or APC string runs to its terminator, payload included.
	"\x1b[PX^_][^\x1b]*\x1b\\\\"

// removed is what Strip and Lines take out of whole text before its control
// characters go: Escape's sequences; an OSC or a device control, privacy or
// application string in its C1 form (U+009D, U+0090 and so on, which some
// terminals read as the escape it stands for), payload and all; an OSC that
// never finished, with the rest of its line, so a title or a link with no
// terminator does not print as text; and the parameters of a CSI that never
// finished. The alternatives are tried in order, so a finished sequence wins
// over the unfinished one that would also match its start.
//
// A lone ESC or C1 control is not a sequence here: it goes as the control
// character it is, and the text after it stays (a\x1bb is ab). Without it
// a terminal acts on none of that text, and reading the next character as
// part of an escape (ESC p, ESC P...) would drop a letter of a name.
var removed = regexp.MustCompile(escapePattern + "|" +
	"\u009d[^\x07\x1b\u009c]*(?:\x07|\x1b\\\\|\u009c)|" +
	"[\u0090\u0098\u009e\u009f][^\x1b\u009c]*(?:\x1b\\\\|\u009c)|" +
	"(?:\x1b\\]|\u009d)[^\x07\x1b\u009c\n]*|" +
	"\x1b\\[[0-9;:?]*[\\x20-\\x2f]*")

// Strip removes escapes, control characters and Misleading characters from
// s. keepSpace keeps the white space among the control characters (a newline,
// a tab) for the caller to collapse or keep.
//
// Text with nothing to remove, the usual case, is returned as it is, without
// allocating.
func Strip(s string, keepSpace bool) string {
	if printable(s, keepSpace) {
		return s
	}
	if keepSpace {
		return drop(withoutEscapes(s), goneKeepingSpace)
	}
	return drop(withoutEscapes(s), gone)
}

// gone is a rune Strip removes: a control character or a Misleading one.
func gone(r rune) bool { return unicode.IsControl(r) || Misleading(r) }

// goneKeepingSpace is gone, but for the white space among the control
// characters.
func goneKeepingSpace(r rune) bool {
	return (unicode.IsControl(r) && !unicode.IsSpace(r)) || Misleading(r)
}

// Lines is text as a multi-line value shows it: without escapes, control
// characters or Misleading characters, but with its line breaks and tabs, so a
// pretty-printed JSON value still reads as one, and a log line keeps its
// tabs. A CRLF line break becomes a plain newline, and a carriage return on
// its own, which would let a later part of the text overwrite an earlier one
// on screen, is dropped.
//
// Text with nothing to remove, the usual case, is returned as it is, without
// allocating.
func Lines(s string) string {
	if printable(s, true) {
		return s
	}
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	return drop(withoutEscapes(s), notShown)
}

// notShown is a rune Lines removes (Shown).
func notShown(r rune) bool { return Shown(r) == -1 }

// printable reports whether s is printable ASCII, with tabs and line breaks
// too when lines is set: text with nothing in it for Strip or Lines to remove,
// the usual log line, found with one pass over its bytes and no rune decoding.
func printable(s string, lines bool) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 0x20 && b < 0x7f || lines && (b == '\t' || b == '\n') {
			continue
		}
		return false
	}
	return true
}

// Shown is r as Lines shows it: a line break and a tab as they are, and a
// control character or a Misleading character as -1, for strings.Map to
// drop.
func Shown(r rune) rune {
	if r == '\n' || r == '\t' {
		return r
	}
	if unicode.IsControl(r) || Misleading(r) {
		return -1
	}
	return r
}

// withoutEscapes is s without what removed matches. The pattern runs only on
// text that has an ESC or a C1 control in it (C1 is 0xC2 followed by a byte
// below 0xA0 in UTF-8), so text without one costs a scan of its bytes.
func withoutEscapes(s string) string {
	for i := 0; i < len(s); i++ {
		if b := s[i]; b == 0x1b || (b == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] < 0xa0) {
			return removed.ReplaceAllString(s, "")
		}
	}
	return s
}

// drop is s without the runes gone reports, and s itself when there are none.
func drop(s string, gone func(rune) bool) string {
	if strings.IndexFunc(s, gone) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if gone(r) {
			return -1
		}
		return r
	}, s)
}

// Misleading is a character with no glyph of its own that reorders the text
// around it or hides in it: the bidirectional controls (an override can show a
// different run ID from the one acted on), the zero-width space, the byte
// order mark, the word joiner and the invisible math operators, the deprecated
// formatting controls and the interlinear annotation marks. The zero-width
// joiner and non-joiner, the variation selectors and the tag characters stay:
// emoji sequences (England's flag among them) and Persian and Indic words are
// written with them.
func Misleading(r rune) bool {
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
