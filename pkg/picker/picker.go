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

// retry is what a picker that asks again prints, before the prompt, after an
// answer that picked nothing.
const retry = "Not one of the choices."

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

	// Default, when above 0, is the number of the row an empty answer (Enter)
	// picks, and the prompt shows it: "> [2] ". At 0 an empty answer picks
	// nothing, like any other answer that is not a row number.
	Default int
	// Attempts is how many answers Pick reads before it fails. Above 0, it
	// says so after every answer that picks nothing, the last included, and
	// asks again while answers remain. At 0 it asks once and says nothing.
	Attempts int
	// ByName lets an answer that is exactly a row's first cell pick that row,
	// ahead of the numbers: a row may be called "2", and what someone typed is
	// what they meant.
	ByName bool
	// Ended, when set, is the error for input that ends (Ctrl-D, a closed
	// stdin) before an answer picks a row: with nothing typed, or with a
	// last answer cut short that picks nothing. Input that has ended cannot
	// be asked again, so it is returned at once. When nil, ended input fails
	// like any answer that picks nothing.
	Ended error

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
// "02", "+2" and " 2" pick nothing. With ByName a row's first cell, exactly,
// picks it too, and with Default an empty line picks that row. Anything else
// is asked again, up to Attempts answers, each wrong one told so, and then
// fails with Invalid (or InvalidAnswer's error, naming the last answer given).
// Input that ends is not asked again: an answer cut short by it is read like
// any other, and if it picks nothing Pick fails with Ended when that is set.
func (l *List) Pick(out io.Writer, in io.Reader) (int, error) {
	if err := input.MayAsk(prompt, l.Ask...); err != nil {
		return 0, err
	}
	l.render(out)
	r := input.Reader(in)
	var last string // the last answer given, for InvalidAnswer to name
	for attempt := 1; ; attempt++ {
		// A failed read ends the input: whatever it returned is the last
		// answer there will be.
		line, readErr := r.ReadString('\n')
		ended := readErr != nil
		answer := strings.Trim(line, "\r\n")
		if i, ok := l.match(answer, !ended); ok {
			return i, nil
		}
		// Input that ended with nothing on its line is no answer at all.
		answered := answer != "" || !ended
		if answered {
			last = answer
		}
		// A picker that asks again tells every wrong answer so, the last
		// included: one that ends quietly would otherwise end with no word.
		tell := answered && l.Attempts > 0
		if tell {
			io.WriteString(out, retry) //nolint:errcheck // a failed write to the terminal still reads the answer
		}
		if ended || attempt >= l.Attempts {
			if tell {
				io.WriteString(out, "\n") //nolint:errcheck // as above
			}
			if ended && l.Ended != nil {
				return 0, l.Ended
			}
			break
		}
		io.WriteString(out, l.prompt()) //nolint:errcheck // as above
	}
	if l.InvalidAnswer != nil {
		return 0, l.InvalidAnswer(last)
	}
	return 0, l.Invalid
}

// match reads one answer: the 0-based row it picks, and whether it picks one.
// An empty answer is Enter, and picks Default, only when its line ended
// (entered); input that ended is not Enter.
func (l *List) match(answer string, entered bool) (int, bool) {
	if answer == "" {
		if entered && l.Default > 0 && l.Default <= len(l.rows) {
			return l.Default - 1, true
		}
		return 0, false
	}
	if l.ByName {
		for i, row := range l.rows {
			if len(row) > 0 && row[0] == answer {
				return i, true
			}
		}
	}
	n, ok := Number(answer, 1, len(l.rows))
	if !ok {
		return 0, false
	}
	return n - 1, true
}

// prompt is the line that asks for the answer, with the row Enter picks in
// brackets when there is one.
func (l *List) prompt() string {
	if l.Default > 0 && l.Default <= len(l.rows) {
		return fmt.Sprintf("%s[%d] ", prompt, l.Default)
	}
	return prompt
}

// Number reads answer the way Pick does, for a prompt that numbers its rows
// itself: it returns the number answer spells, and true, only when that is
// one of first to last written exactly. "2" is 2, but "02", "+2", " 2" and
// "2.0" are no number at all.
func Number(answer string, first, last int) (int, bool) {
	n, err := strconv.Atoi(answer)
	if err != nil || n < first || n > last || strconv.Itoa(n) != answer {
		return 0, false
	}
	return n, true
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
	b.WriteString(l.prompt()) //nolint:errcheck // sticky: reported, and ignored, at the Flush
	b.Flush()                 //nolint:errcheck // a failed write to the terminal still reads the answer
}
