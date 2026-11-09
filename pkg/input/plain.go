package input

import (
	"regexp"
	"strings"
)

// ansiEscape is a terminal escape sequence: a CSI (the styles ansi.Bold wraps
// a name in, cursor controls with private parameters) or an OSC (a hyperlink,
// a title), as pkg/localrt strips from logs.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[\\x20-\\x2f]*[\\x40-\\x7e]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// plain is a prompt as a message quotes it: without the escapes it was styled
// with, which %q would print as text (\x1b[1m), and on one line. The caller
// still quotes it with %q, which escapes anything left.
func plain(prompt string) string {
	return strings.Join(strings.Fields(ansiEscape.ReplaceAllString(prompt, "")), " ")
}
