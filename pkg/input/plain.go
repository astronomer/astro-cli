package input

import (
	"regexp"
	"strings"
)

// ansiEscape is a terminal escape sequence, as ansi.Bold wraps a name in.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")

// plain is a prompt as a message quotes it: without the escapes it was styled
// with, which %q would print as text (\x1b[1m), and on one line.
func plain(prompt string) string {
	return strings.Join(strings.Fields(ansiEscape.ReplaceAllString(prompt, "")), " ")
}
