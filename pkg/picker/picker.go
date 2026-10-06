// Package picker asks a person to choose one row of a numbered table: the
// one way every interactive picker in the CLI asks.
//
// It lives in pkg/, not cmd/, because most pickers are asked from deep in the
// platform packages under internal/, which may not import cmd/. Its table is
// pkg/texttable's, the one cmd/cliout's lists print, so a picker's rows line up
// the way a list's do.
package picker

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/texttable"
)

// prompt is what a picker prints to ask for its answer, after the table.
const prompt = "\n> "

// The highlight a current row is drawn in: bold green.
const (
	highlightOn  = "\033[1;32m"
	highlightOff = "\033[0m"
)

// List is one question: a table of rows, numbered from 1 in a "#" column the
// picker adds in front of the caller's, and the number of one of them read
// back as the answer.
type List struct {
	// Title, when not "", is printed on its own line above the table.
	Title string
	// Header names the caller's columns; the "#" column goes in front.
	Header []string
	// Ask describes the question to a run that may not ask it: input.About
	// for what is asked, input.AnsweredBy for the flag or argument that
	// answers it instead.
	Ask []input.Option
	// Invalid is the error for an answer that is not one of the row numbers.
	Invalid error
	// InvalidAnswer, when set, is used instead of Invalid, for a picker whose
	// error names the answer it refused.
	InvalidAnswer func(answer string) error

	rows    [][]string
	current int // the highlighted row, 1-based; 0 for none
}

// AddRow appends a row of cells, one per Header column. A current row (the
// workspace or context in use now) is drawn bold green, so the reader sees
// what they are choosing away from; only the last one marked is.
func (l *List) AddRow(current bool, cells ...string) {
	l.rows = append(l.rows, cells)
	if current {
		l.current = len(l.rows)
	}
}

// Pick asks the question. It checks first that this run may ask (see
// input.MayAsk), and refuses with the *input.RequiredError, having written
// nothing, when it may not. Otherwise it writes the title, the table and the
// prompt to out, and reads one line from in. It returns the 0-based index of
// the row whose number that line is, exactly: "2" picks the second row, but
// "02", "+2" and " 2" pick nothing. Anything but a row number fails with
// Invalid (or InvalidAnswer's error).
func (l *List) Pick(out io.Writer, in io.Reader) (int, error) {
	if err := input.MayAsk(prompt, l.Ask...); err != nil {
		return 0, err
	}
	l.render(out)
	line, _ := bufio.NewReader(in).ReadString('\n') //nolint:errcheck // a failed read is an answer that picks nothing
	answer := strings.Trim(line, "\r\n")
	n, err := strconv.Atoi(answer)
	if err != nil || n < 1 || n > len(l.rows) || strconv.Itoa(n) != answer {
		if l.InvalidAnswer != nil {
			return 0, l.InvalidAnswer(answer)
		}
		return 0, l.Invalid
	}
	return n - 1, nil
}

// render writes the question: the title, the table with its current row
// highlighted, and the prompt. A failed write to the terminal still reads an
// answer, as a prompt always has, so its error goes nowhere.
func (l *List) render(out io.Writer) {
	tab := texttable.Table{Header: append([]string{"#"}, l.Header...)}
	for i, row := range l.rows {
		tab.AddRow(append([]string{strconv.Itoa(i + 1)}, row...)...)
	}
	var table bytes.Buffer
	tb := bufio.NewWriter(&table)
	tab.Render(tb)
	tb.Flush() //nolint:errcheck // a bytes.Buffer does not fail

	b := bufio.NewWriter(out)
	if l.Title != "" {
		fmt.Fprintln(b, l.Title)
	}
	// Line 0 is the header, so row i is line i. The highlight wraps the whole
	// line, after the table is laid out: inside a cell, tabwriter would count
	// the escape codes as width and push that row's columns out of line.
	for i, line := range strings.SplitAfter(table.String(), "\n") {
		if i > 0 && i == l.current {
			line = highlightOn + strings.TrimSuffix(line, "\n") + highlightOff + "\n"
		}
		b.WriteString(line) //nolint:errcheck // sticky: reported, and ignored, at the Flush
	}
	b.WriteString(prompt) //nolint:errcheck // sticky: reported, and ignored, at the Flush
	b.Flush()             //nolint:errcheck // a failed write to the terminal still reads the answer
}
