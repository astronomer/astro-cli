package input

import (
	"strings"

	"github.com/astronomer/astro-cli/pkg/termtext"
)

// The rules for what a terminal is given of text from elsewhere (which escape
// sequences, control characters and invisible characters go) are pkg/termtext's,
// shared with the local runtime's log lines, so the CLI and the runtime show
// the same text. The functions here are the shapes the CLI prints it in.

// Plain is text as a message or a log line shows it: without the escapes it
// was styled with, without any other control character (a lone ESC, a bell,
// a backspace) or invisible character that makes text read as something it is
// not (termtext.Misleading), and on one line. Other format characters stay: the
// zero-width joiner and non-joiner and the variation selectors are part of
// emoji sequences and of Persian and Indic words. It is how a prompt is quoted
// in a refusal, and how astro local's [y/N] reads and echoes an answer from a
// pipe.
func Plain(s string) string {
	return strings.Join(strings.Fields(termtext.Strip(s, true)), " ")
}

// Clean is s without escapes, control characters (a newline or a tab
// included) or termtext.Misleading characters, and otherwise as it was: not
// trimmed, and with every space kept, so two names that differ in their
// spacing still differ. It is for text that names something on this machine
// (a project's path in a question) and must not be altered, only made safe to
// print.
func Clean(s string) string { return termtext.Strip(s, false) }

// Cell is text as one table cell shows it: a line break, a tab, a vertical
// tab or a form feed becomes a space (each of them would end the cell or the
// row), a carriage return goes, and then escapes, control characters and
// termtext.Misleading characters go, as Clean removes them. Unlike Plain it
// keeps the text's own spacing: a name with two spaces in it still shows two.
// It is for a value from somewhere else (an API's name or description) in a
// table.
func Cell(s string) string {
	return termtext.Strip(cellSpace.Replace(s), false)
}

// cellSpace is the white space Cell turns into a space before it strips.
var cellSpace = strings.NewReplacer("\r\n", " ", "\n", " ", "\t", " ", "\v", " ", "\f", " ", "\r", "")

// Lines is text as a multi-line value shows it: without escapes, control
// characters or termtext.Misleading characters, but with its line breaks and
// tabs, so a pretty-printed JSON value still reads as one. A CRLF line break
// becomes a plain newline, and a carriage return on its own, which would let a
// later part of the text overwrite an earlier one on screen, is dropped. It is
// for text from somewhere else (an Airflow API response) printed as it is.
func Lines(s string) string { return termtext.Lines(s) }

// Quote is prompt in double quotes, as a message names a question: its Plain
// text with a literal double quote escaped and nothing else. Not %q, which
// would also double every backslash, so a Windows path in the question
// (C:\Users\me\proj) would read C:\\Users\\me\\proj, and again in json.
func Quote(prompt string) string { return quotePlain(Plain(prompt)) }

// quotePlain is Quote for text that is already Plain.
func quotePlain(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
