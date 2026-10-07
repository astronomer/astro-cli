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
	"errors"
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

// DefaultAttempts is how many answers a picker reads before it fails, unless
// its List says otherwise: a typo is asked again, but a stdin that answers
// and never answers usefully still ends.
const DefaultAttempts = 3

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
	// Empty is the error for a List with no rows and no Keys: nothing to
	// choose from, which is not a bad answer, since none was given. When nil
	// the error is ErrNoChoices, naming what Ask says is asked about.
	Empty error

	// Default, when above 0, is the number of the row an empty answer (Enter)
	// picks, and the prompt shows it: "> [2] ". At 0 an empty answer picks
	// nothing, like any other answer that is not a row number.
	Default int
	// Attempts is how many answers Pick reads before it fails; at 0 it is
	// DefaultAttempts. It says so after every answer that picks nothing, the
	// last included, and asks again while answers remain. 1 asks once.
	Attempts int
	// Keys are answers besides the row numbers that the question takes, such
	// as "n" for a next page: an answer that is exactly one of them is that
	// key, which Choose returns in place of a row. Hint says what they mean.
	// A key is read before the numbers. A List with Keys is asked with
	// Choose: Pick has no way to return a key. With ByName no row may be
	// named like a key, since one answer cannot mean both: Choose panics on
	// such a List, a programming error.
	Keys []string
	// Hint, when not "", is printed on its own line between the table and the
	// prompt, and again with the prompt each time it is asked again.
	Hint string
	// First is the number the first row is shown with and answered by, for a
	// page of a longer list numbered on from the pages before it; 0 is 1.
	First int
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
// A List with Keys is asked with Choose: Pick panics on one, a programming
// error that would otherwise read a key as the first row.
func (l *List) Pick(out io.Writer, in io.Reader) (int, error) {
	if len(l.Keys) > 0 {
		panic("picker: a List with Keys is asked with Choose, not Pick")
	}
	c, err := l.Choose(out, in)
	return c.Row, err
}

// Choice is the answer Choose read: one of the List's Keys, or, when Key is
// "", the 0-based index of a row.
type Choice struct {
	Row int
	Key string
}

// Choose asks the question as Pick does, and returns the answer: a row, or
// one of Keys answered exactly. A List with no rows and no Keys has no answer
// to give, so on a run that may ask it fails at once with Invalid, printing
// and reading nothing; a run that may not ask is refused as it always is.
func (l *List) Choose(out io.Writer, in io.Reader) (Choice, error) {
	if err := l.MayAsk(); err != nil {
		return Choice{}, err
	}
	if len(l.rows) == 0 && len(l.Keys) == 0 {
		return Choice{}, l.empty()
	}
	l.mustNotShadowKeys()
	l.render(out)
	attempts := l.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	r := input.Reader(in)
	var last string // the last answer given, for InvalidAnswer to name
	for attempt := 1; ; attempt++ {
		// A failed read ends the input: whatever it returned is the last
		// answer there will be.
		line, readErr := r.ReadString('\n')
		ended := readErr != nil
		answer := strings.Trim(line, "\r\n")
		if c, ok := l.match(answer, !ended); ok {
			return c, nil
		}
		// Input that ended with nothing on its line is no answer at all.
		answered := answer != "" || !ended
		if answered {
			last = answer
			// Every wrong answer is told so, the last included: one that
			// ends quietly would otherwise end with no word.
			io.WriteString(out, retry) //nolint:errcheck // a failed write to the terminal still reads the answer
		}
		if ended || attempt >= attempts {
			if answered {
				io.WriteString(out, "\n") //nolint:errcheck // as above
			}
			if ended && l.Ended != nil {
				return Choice{}, l.Ended
			}
			break
		}
		io.WriteString(out, l.prompt()) //nolint:errcheck // as above
	}
	return Choice{}, l.invalid(last)
}

// MayAsk is the check Choose makes before it prints anything: whether this
// run may ask the question (see input.MayAsk). A caller that has to fetch the
// rows first makes it before fetching, so a run that may not ask fetches
// nothing, and refuses with the error Choose would.
func (l *List) MayAsk() error {
	return input.MayAsk(prompt, l.Ask...)
}

// ErrNoChoices is what a List with nothing to choose from fails with, unless
// its Empty says otherwise.
var ErrNoChoices = errors.New("there is nothing to choose from")

// empty is the error for a List with nothing to choose from: Empty, or
// ErrNoChoices naming what Ask says is asked about.
func (l *List) empty() error {
	if l.Empty != nil {
		return l.Empty
	}
	var about input.RequiredError
	for _, o := range l.Ask {
		o(&about)
	}
	if about.About == "" {
		return ErrNoChoices
	}
	return fmt.Errorf("%w: no choice of %s", ErrNoChoices, about.About)
}

// mustNotShadowKeys panics on a List whose ByName rows are named like one of
// its Keys: an answer that is that name would mean the row and the key both.
func (l *List) mustNotShadowKeys() {
	if !l.ByName {
		return
	}
	for _, k := range l.Keys {
		for _, row := range l.rows {
			if len(row) > 0 && row[0] == k {
				panic(fmt.Sprintf("picker: a ByName row is named %q, one of its Keys", k))
			}
		}
	}
}

// invalid is the error for answers that picked nothing, the last of them
// answer.
func (l *List) invalid(answer string) error {
	if l.InvalidAnswer != nil {
		return l.InvalidAnswer(answer)
	}
	return l.Invalid
}

// match reads one answer: the choice it makes, and whether it makes one. An
// empty answer is Enter, and picks Default, only when its line ended
// (entered); input that ended is not Enter.
func (l *List) match(answer string, entered bool) (Choice, bool) {
	if answer == "" {
		if entered && l.Default > 0 && l.Default <= len(l.rows) {
			return Choice{Row: l.Default - 1}, true
		}
		return Choice{}, false
	}
	if l.ByName {
		for i, row := range l.rows {
			if len(row) > 0 && row[0] == answer {
				return Choice{Row: i}, true
			}
		}
	}
	for _, k := range l.Keys {
		if k == answer {
			return Choice{Key: k}, true
		}
	}
	first := l.first()
	n, ok := number(answer, first, first+len(l.rows)-1)
	if !ok {
		return Choice{}, false
	}
	return Choice{Row: n - first}, true
}

// first is the number the first row is shown with.
func (l *List) first() int {
	if l.First > 0 {
		return l.First
	}
	return 1
}

// prompt is what asks for the answer: the Hint, when there is one, and the
// prompt line, with the row Enter picks in brackets when there is one.
func (l *List) prompt() string {
	var hint string
	if l.Hint != "" {
		hint = "\n" + l.Hint
	}
	if l.Default > 0 && l.Default <= len(l.rows) {
		// Default counts rows from 1; the prompt shows the number the row
		// is shown and answered with.
		return fmt.Sprintf("%s%s[%d] ", hint, prompt, l.first()+l.Default-1)
	}
	return hint + prompt
}

// number reads answer the way Pick does: it returns the number answer spells,
// and true, only when that is one of first to last written exactly. "2" is 2,
// but "02", "+2", " 2" and "2.0" are no number at all.
func number(answer string, first, last int) (int, bool) {
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
	first := l.first()
	for i, row := range l.rows {
		tab.AddRow(append([]string{strconv.Itoa(first + i)}, row...)...)
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
