package input

import (
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// sgr is a Select Graphic Rendition sequence, ESC [ params m: color, bold,
// reset. It is the one escape the CLI's own output uses (ansi.Bold, a table's
// highlighted row, colored json), and the one Terminal keeps. It changes how
// text looks and nothing else: it cannot move the cursor, clear the screen,
// set a title or the clipboard.
var sgr = regexp.MustCompile("^\x1b\\[[0-9;]*m$")

// ansiEscapeAtStart is ansiEscape anchored at the start of its input, for
// TerminalWriter to tell a complete sequence from one a later write finishes.
var ansiEscapeAtStart = regexp.MustCompile("^(?:" + ansiEscape.String() + ")")

// Terminal is text as a terminal is given it: with the SGR styling it carries
// (sgr), and without anything else a terminal acts on rather than shows. That
// means other escape sequences (a clipboard write or a title (OSC), a cursor
// move or a screen clear (CSI), a device control string), a lone ESC, any other
// control character (a bell, a backspace, a carriage return that would let a
// later part of a line overwrite an earlier one) and misleadingRune
// characters. Line breaks and tabs stay, and a CRLF line break becomes a
// newline.
//
// It is the filter for output whose text came at least in part from
// elsewhere (an API's names, a Deployment's logs) and whose styling is the
// CLI's own: what Lines does, keeping the colors.
func Terminal(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, m := range ansiEscape.FindAllStringIndex(s, -1) {
		b.WriteString(terminalText(s[last:m[0]]))
		if seq := s[m[0]:m[1]]; sgr.MatchString(seq) {
			b.WriteString(seq)
		}
		last = m[1]
	}
	b.WriteString(terminalText(s[last:]))
	return b.String()
}

// terminalText is a stretch of Terminal's input between escape sequences:
// control characters but a line break and a tab, and misleadingRune
// characters, removed.
func terminalText(s string) string {
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

// esc is the byte every escape sequence starts with.
const esc = 0x1b

// maxHeldEscape is how much of an unfinished escape sequence a TerminalWriter
// holds back for the write that finishes it. A real sequence is far shorter;
// past this the bytes are released through Terminal, which drops the ESC that
// would have made them a sequence, so nothing is held without bound.
const maxHeldEscape = 64 << 10

// TerminalWriter writes what it is given to W as Terminal has it, as it
// arrives: a command's rendering can be long, or can stream, so nothing waits
// for the end. An escape sequence or a UTF-8 character that one Write splits
// from the next is held until the rest arrives. Flush releases whatever is
// still held, once the writing is done.
type TerminalWriter struct {
	W       io.Writer
	pending []byte
}

// Write filters p (with anything held from before) and writes the result to
// W. It reports len(p) on success, as the bytes were all taken.
func (t *TerminalWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(t.pending)+len(p))
	buf = append(buf, t.pending...)
	buf = append(buf, p...)
	cut := heldFrom(buf)
	if _, err := io.WriteString(t.W, Terminal(string(buf[:cut]))); err != nil {
		return 0, err
	}
	t.pending = append(t.pending[:0], buf[cut:]...)
	return len(p), nil
}

// Flush ends the writing. What is still held is either an escape sequence no
// write finished, which is dropped whole, or the start of a character no
// write finished, which is removed.
func (t *TerminalWriter) Flush() error {
	held := t.pending
	t.pending = t.pending[:0]
	if len(held) == 0 || held[0] == esc {
		return nil
	}
	_, err := io.WriteString(t.W, Terminal(strings.ToValidUTF8(string(held), "")))
	return err
}

// heldFrom is where in buf the bytes a later write may finish begin: an escape
// sequence started but not yet complete, or the leading bytes of a UTF-8
// character. All of buf is released when it holds neither.
func heldFrom(buf []byte) int {
	cut := len(buf)
	// The last ESC, if what follows it is not yet a whole sequence. One whose
	// sequence is complete, or that is too far back to be one, is not held.
	if i := strings.LastIndexByte(string(buf), esc); i >= 0 && len(buf)-i <= maxHeldEscape {
		if !ansiEscapeAtStart.Match(buf[i:]) && couldContinue(buf[i:]) {
			cut = i
		}
	}
	// A UTF-8 character cut short at the end.
	for k := cut - 1; k >= 0 && k >= cut-utf8.UTFMax; k-- {
		if utf8.RuneStart(buf[k]) {
			if !utf8.FullRune(buf[k:cut]) {
				cut = k
			}
			break
		}
	}
	return cut
}

// couldContinue reports whether seq, which starts with ESC and is not a whole
// sequence, could become one with more bytes: an ESC alone, or the start of a
// CSI, an OSC or a device control string that has not reached its end. Any
// other ESC is not the start of a sequence ansiEscape knows, and is released
// for Terminal to drop.
func couldContinue(seq []byte) bool {
	if len(seq) == 1 {
		return true
	}
	switch seq[1] {
	case '[':
		// Parameters and intermediates so far; a final byte would have
		// completed it.
		for _, c := range seq[2:] {
			if c < 0x20 || c > 0x3f {
				return false
			}
		}
		return true
	case ']', 'P', 'X', '^', '_':
		return true
	}
	return false
}
