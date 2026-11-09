package input

import (
	"regexp"
	"strings"
	"unicode"
)

// ansiEscape is a terminal escape sequence: a CSI (the styles ansi.Bold wraps
// a name in, cursor controls with private parameters) or an OSC (a hyperlink,
// a title), as pkg/localrt strips from logs.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[\\x20-\\x2f]*[\\x40-\\x7e]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// Plain is text as a message or a log line shows it: without the escapes it
// was styled with, without any other control character (a lone ESC, a bell,
// a backspace) or invisible format character (a right-to-left override, a
// zero-width space, which would make it read as something it is not), and on
// one line. It is how a prompt is quoted in a refusal, and how astro local's
// [y/N] reads and echoes an answer from a pipe.
func Plain(s string) string {
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && !unicode.IsSpace(r)) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
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
