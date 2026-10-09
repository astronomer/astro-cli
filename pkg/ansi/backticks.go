package ansi

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
)

// The CLI's messages are written the way its docs are: a command, flag or path
// a person might type goes in backticks, as in "Use `astro local start`
// instead". The source keeps them, and they are rendered where a message
// leaves the CLI, not where it is written:
//
//   - to a terminal with color on, the span is bold and the backticks go;
//   - anywhere else (a pipe, a file, NO_COLOR, a test's buffer), only the
//     backticks go, leaving plain text;
//   - in a json payload, the same plain text (StripBackticks).
//
// Only a span that reads as markup is touched, so a backtick that is content
// (one inside a word or a quoted value, an empty pair, a run of three) is
// left as it is. A span is a backtick that does not follow a letter, digit or
// backtick, then up to maxSpanRunes of text on one line that neither starts
// nor ends with a space, then a backtick that no letter, digit or backtick
// follows. Text whose backticks are content throughout, such as a shell line
// that uses them, does not go through it at all.

// maxSpanRunes is the longest span rendered. The longest the CLI writes is a
// whole command line with its flags; anything longer is more likely two
// unrelated backticks than one span.
const maxSpanRunes = 200

// ForWriter is the palette for text written to w: colored when w is a
// terminal and color is on (pkg/ansi's switches: CLICOLOR_FORCE, CLICOLOR,
// NO_COLOR, ForceColors), and plain when w is anything else, a buffer or a
// pipe.
func ForWriter(w io.Writer) Palette {
	return Palette{aurora.NewAurora(shouldColor(func() bool { return isTerminal(w) }))}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// Backticks renders s's backticked spans for w (see ForWriter): bold on a
// terminal, plain text elsewhere. Every message a command writes to a person
// goes through it, or through the error and help rendering that calls it.
func Backticks(w io.Writer, s string) string { return ForWriter(w).Backticks(s) }

// Backticks renders s's backticked spans in the palette: bold when it colors,
// plain text when it does not. The backticks themselves are dropped either way.
func (p Palette) Backticks(s string) string { return replaceSpans(s, p.Bold) }

// StripBackticks drops the backticks around s's spans, leaving plain text: what
// a json payload carries, and what a terminal shows with color off.
func StripBackticks(s string) string { return replaceSpans(s, func(t string) string { return t }) }

// Fprintf writes the formatted message to w with its backticked spans rendered
// for w: the notes, warnings and hints a command prints itself, rather than
// returning as an error (which cliout.Execute renders) or handing to a
// renderer that does.
func Fprintf(w io.Writer, format string, args ...any) (int, error) {
	return io.WriteString(w, Backticks(w, fmt.Sprintf(format, args...)))
}

func replaceSpans(s string, render func(string) string) string {
	if !strings.Contains(s, "`") {
		return s
	}
	var b strings.Builder
	rest := 0 // s[rest:] is not written yet
	for i := 0; i < len(s); i++ {
		if s[i] != '`' {
			continue
		}
		end, ok := spanAt(s, i)
		if !ok {
			continue
		}
		b.WriteString(s[rest:i])
		b.WriteString(render(s[i+1 : end]))
		rest = end + 1
		i = end
	}
	if rest == 0 {
		return s
	}
	b.WriteString(s[rest:])
	return b.String()
}

// spanAt reports whether the backtick at s[open] opens a span, and the index
// of the backtick that closes it.
func spanAt(s string, open int) (closing int, ok bool) {
	if open > 0 {
		if prev, _ := utf8.DecodeLastRuneInString(s[:open]); prev == '`' || isWordRune(prev) {
			return 0, false
		}
	}
	rel := strings.IndexAny(s[open+1:], "`\n")
	if rel <= 0 || s[open+1+rel] != '`' {
		// No closing backtick on this line, or an empty ``.
		return 0, false
	}
	closing = open + 1 + rel
	content := s[open+1 : closing]
	if strings.TrimSpace(content) != content || utf8.RuneCountInString(content) > maxSpanRunes {
		return 0, false
	}
	if closing+1 < len(s) {
		if next, _ := utf8.DecodeRuneInString(s[closing+1:]); next == '`' || isWordRune(next) {
			return 0, false
		}
	}
	return closing, true
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// VisibleWidth is the number of runes s shows on a terminal: its length without
// the escape sequences that color it.
func VisibleWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}
