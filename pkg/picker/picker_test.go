package picker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
)

var errInvalidThing = errors.New("invalid thing selection")

func things() *List {
	l := &List{
		Title:   "Which thing?",
		Header:  []string{"NAME", "ID"},
		Ask:     []input.Option{input.About("a thing"), input.AnsweredBy("--thing")},
		Invalid: errInvalidThing,
	}
	l.AddRow(false, "dev", "d1")
	l.AddRow(true, "prod", "p1")
	l.AddRow(false, "staging", "s1")
	return l
}

// A row number picks that row, after the title, the numbered table and the
// prompt are written.
func TestPickReadsARowNumber(t *testing.T) {
	var out bytes.Buffer
	i, err := things().Pick(&out, strings.NewReader("3\n"))
	require.NoError(t, err)
	assert.Equal(t, 2, i)

	got := out.String()
	assert.True(t, strings.HasPrefix(got, "Which thing?\n"), got)
	assert.True(t, strings.HasSuffix(got, "\n\n> "), "%q", got)
	lines := strings.Split(strings.TrimSuffix(got, "\n\n> "), "\n")
	require.Len(t, lines, 5, "title, header, three rows")
	assert.Equal(t, []string{"#", "NAME", "ID"}, strings.Fields(lines[1]), "the # column goes first")
	assert.Equal(t, []string{"1", "dev", "d1"}, strings.Fields(lines[2]))
	assert.Equal(t, []string{"3", "staging", "s1"}, strings.Fields(lines[4]))
	assert.Equal(t, strings.Index(lines[1], "ID"), strings.Index(lines[4], "s1"), "the columns line up")

	_, err = things().Pick(&bytes.Buffer{}, strings.NewReader("1\r\n"))
	require.NoError(t, err, "a CRLF answer")
	_, err = things().Pick(&bytes.Buffer{}, strings.NewReader("1"))
	require.NoError(t, err, "an answer with no line end")
}

// The current row is wrapped whole in bold green; the others are plain, and
// the highlight does not move the row's columns.
func TestPickHighlightsTheCurrentRow(t *testing.T) {
	var out bytes.Buffer
	_, err := things().Pick(&out, strings.NewReader("1\n"))
	require.NoError(t, err)
	lines := strings.Split(out.String(), "\n")
	assert.Equal(t, 1, strings.Count(out.String(), highlightOn), "one row is highlighted")
	prod := lines[3]
	require.True(t, strings.HasPrefix(prod, highlightOn) && strings.HasSuffix(prod, highlightOff), "%q", prod)
	inner := strings.TrimSuffix(strings.TrimPrefix(prod, highlightOn), highlightOff)
	assert.Equal(t, len(lines[2]), len(inner), "the highlighted row is laid out like the rest")
	assert.NotContains(t, lines[2], "\033", "a row not current is plain")

	out.Reset()
	l := &List{Header: []string{"NAME"}, Invalid: errInvalidThing}
	l.AddRow(false, "only")
	_, err = l.Pick(&out, strings.NewReader("1\n"))
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "\033", "no current row, no highlight")
	assert.True(t, strings.HasPrefix(out.String(), " #"), "an empty title prints no line: %q", out.String())
}

// Anything but one of the row numbers, spelled exactly, picks nothing and
// fails with the caller's own error.
func TestPickRefusesAnythingButARowNumber(t *testing.T) {
	for _, answer := range []string{"", "\n", "0\n", "4\n", "-1\n", "prod\n", " 1\n", "1 \n", "01\n", "+1\n", "1.0\n", "0x1\n"} {
		_, err := things().Pick(&bytes.Buffer{}, strings.NewReader(answer))
		assert.ErrorIs(t, err, errInvalidThing, "answer %q", answer)
	}
}

// InvalidAnswer, when set, builds the error from what was answered.
func TestPickInvalidAnswerNamesTheAnswer(t *testing.T) {
	l := things()
	l.InvalidAnswer = func(answer string) error { return fmt.Errorf("%w: %s selected", errInvalidThing, answer) }
	_, err := l.Pick(&bytes.Buffer{}, strings.NewReader("9\n"))
	require.ErrorIs(t, err, errInvalidThing)
	assert.EqualError(t, err, "invalid thing selection: 9 selected")
}

// A run that may not ask refuses before it writes or reads anything, naming
// what was asked and what answers it.
func TestPickRefusesWithoutWritingWhenItMayNotAsk(t *testing.T) {
	restore := input.SetGuard(func() string { return "with --output json it cannot" })
	defer restore()

	var out bytes.Buffer
	in := strings.NewReader("1\n")
	_, err := things().Pick(&out, in)
	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "a thing")
	assert.Contains(t, err.Error(), "--thing")
	assert.Empty(t, out.String(), "nothing written")
	assert.Equal(t, 2, in.Len(), "nothing read")
}

// Default makes Enter an answer, and the prompt shows which row it picks.
// Input that ends is not Enter, and picks nothing.
func TestPickDefaultOnEnter(t *testing.T) {
	l := things()
	l.Default = 2
	var out bytes.Buffer
	i, err := l.Pick(&out, strings.NewReader("\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, i)
	assert.True(t, strings.HasSuffix(out.String(), "\n\n> [2] "), "%q", out.String())

	l = things()
	l.Default = 2
	i, err = l.Pick(&bytes.Buffer{}, strings.NewReader("3\n"))
	require.NoError(t, err)
	assert.Equal(t, 2, i, "a row number still picks its row")

	l = things()
	l.Default = 2
	_, err = l.Pick(&bytes.Buffer{}, strings.NewReader(""))
	require.ErrorIs(t, err, errInvalidThing, "ended input is not Enter")

	_, err = things().Pick(&bytes.Buffer{}, strings.NewReader("\n"))
	assert.ErrorIs(t, err, errInvalidThing, "with no Default, Enter picks nothing")
}

// Attempts asks again after each answer that picks nothing, saying so, and
// fails with the last answer once they run out. The answers are read as
// strictly on the last try as on the first.
func TestPickAsksAgainThenFails(t *testing.T) {
	l := things()
	l.Default = 1
	var out bytes.Buffer
	i, err := l.Pick(&out, strings.NewReader("9\n 2\n2\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, i)
	assert.Equal(t, 2, strings.Count(out.String(), "Not one of the choices.\n> [1] "), "%q", out.String())

	l = things()
	l.InvalidAnswer = func(answer string) error { return fmt.Errorf("%w: %q", errInvalidThing, answer) }
	out.Reset()
	_, err = l.Pick(&out, strings.NewReader("0\n02\n+2\n1\n"))
	require.ErrorIs(t, err, errInvalidThing)
	require.EqualError(t, err, `invalid thing selection: "+2"`, "the last answer is the one named, and a fourth is not read")
	assert.Equal(t, 2, strings.Count(out.String(), "Not one of the choices.\n> "), "%q", out.String())

	l = things()
	out.Reset()
	_, err = l.Pick(&out, strings.NewReader("9\n"))
	require.ErrorIs(t, err, errInvalidThing)
	assert.Equal(t, 1, strings.Count(out.String(), "Not one of the choices."), "ended input is not asked again")
}

// ByName lets a row's first cell, exactly, pick it, ahead of the numbers.
func TestPickByName(t *testing.T) {
	l := &List{Header: []string{"NAME"}, Invalid: errInvalidThing, ByName: true}
	l.AddRow(false, "2")
	l.AddRow(false, "prod")
	for answer, want := range map[string]int{"2\n": 0, "prod\n": 1, "1\n": 0} {
		i, err := l.Pick(&bytes.Buffer{}, strings.NewReader(answer))
		require.NoError(t, err, answer)
		assert.Equal(t, want, i, "answer %q", answer)
	}
	for _, answer := range []string{" prod\n", "prod \n", "Prod\n", "\n"} {
		_, err := l.Pick(&bytes.Buffer{}, strings.NewReader(answer))
		require.ErrorIs(t, err, errInvalidThing, "answer %q", answer)
	}

	_, err := things().Pick(&bytes.Buffer{}, strings.NewReader("dev\n"))
	assert.ErrorIs(t, err, errInvalidThing, "without ByName a name picks nothing")
}

// Ended is returned at once for input that ends with no answer, without
// asking again; an answer on a last, unterminated line is still read.
func TestPickEnded(t *testing.T) {
	errEnded := errors.New("ended")
	l := things()
	l.Default = 2
	l.Ended = errEnded
	var out bytes.Buffer
	_, err := l.Pick(&out, strings.NewReader("9\n"))
	require.ErrorIs(t, err, errEnded)
	assert.Equal(t, 1, strings.Count(out.String(), "Not one of the choices."))

	i, err := l.Pick(&bytes.Buffer{}, strings.NewReader("3"))
	require.NoError(t, err)
	assert.Equal(t, 2, i)

	// An answer cut short by the end of input that picks nothing cannot be
	// asked again: Ended, after it is told so.
	for _, in := range []string{"prd", "9\nprd"} {
		out.Reset()
		_, err = l.Pick(&out, strings.NewReader(in))
		require.ErrorIs(t, err, errEnded, "input %q", in)
		assert.Equal(t, strings.Count(in, "\n")+1, strings.Count(out.String(), "Not one of the choices.\n"), "input %q: %q", in, out.String())
	}
}

// A picker says so after every wrong answer, the last included, so one that
// runs out does not end without a word. By default it reads DefaultAttempts
// answers; Attempts 1 asks once, and still says so.
func TestPickTellsEveryWrongAnswer(t *testing.T) {
	var out bytes.Buffer
	_, err := things().Pick(&out, strings.NewReader("9\n8\n7\n1\n"))
	require.ErrorIs(t, err, errInvalidThing, "a fourth answer is not read")
	assert.Equal(t, DefaultAttempts, strings.Count(out.String(), "Not one of the choices.\n"), "%q", out.String())
	assert.True(t, strings.HasSuffix(out.String(), "Not one of the choices.\n"), "%q", out.String())

	out.Reset()
	l := things()
	l.Attempts = 1
	_, err = l.Pick(&out, strings.NewReader("9\n1\n"))
	require.ErrorIs(t, err, errInvalidThing)
	assert.Equal(t, 1, strings.Count(out.String(), "Not one of the choices."), "%q", out.String())
	assert.True(t, strings.HasSuffix(out.String(), "Not one of the choices.\n"), "%q", out.String())
}

// pages is the second page of a paged list: rows numbered on from the page
// before, and letters that turn the page.
func pages() *List {
	l := &List{
		Header:  []string{"NAME"},
		Invalid: errInvalidThing,
		Keys:    []string{"p", "n"},
		Hint:    "p. previous n. next",
		First:   4,
	}
	l.AddRow(false, "four")
	l.AddRow(false, "five")
	return l
}

// Pick cannot return a key, so a List with Keys is a programming error there.
func TestPickPanicsOnAListWithKeys(t *testing.T) {
	assert.PanicsWithValue(t, "picker: a List with Keys is asked with Choose, not Pick", func() {
		_, _ = pages().Pick(&bytes.Buffer{}, strings.NewReader("n\n"))
	})
}

// A List with no rows and no Keys has no answer to give: it fails at once,
// printing and reading nothing, with Empty, or ErrNoChoices naming what is
// asked about, and never with the bad-selection error, since no selection
// was made. A run that may not ask is still refused as such, so a script
// learns which flag answers it.
func TestChooseFailsAtOnceWithNothingToChoose(t *testing.T) {
	l := &List{Header: []string{"NAME"}, Invalid: errInvalidThing, Ask: []input.Option{input.About("a thing"), input.AnsweredBy("--thing")}}
	restore := input.SetGuard(func() string { return "with --output json it cannot" })
	_, err := l.Pick(&bytes.Buffer{}, strings.NewReader("1\n"))
	restore()
	require.True(t, input.IsRequired(err), "%v", err)

	var out bytes.Buffer
	in := strings.NewReader("1\n")
	_, err = l.Pick(&out, in)
	require.ErrorIs(t, err, ErrNoChoices)
	assert.EqualError(t, err, "there is nothing to choose from: no choice of a thing")
	assert.Empty(t, out.String())
	assert.Equal(t, 2, in.Len(), "nothing read")

	l.InvalidAnswer = func(answer string) error { return fmt.Errorf("%w: %q", errInvalidThing, answer) }
	_, err = l.Pick(&out, in)
	require.ErrorIs(t, err, ErrNoChoices, "not a bad selection")

	l.Ask = nil
	_, err = l.Pick(&out, in)
	require.EqualError(t, err, ErrNoChoices.Error())

	errNoThings := errors.New("no things in this project")
	l.Empty = errNoThings
	_, err = l.Pick(&out, in)
	require.ErrorIs(t, err, errNoThings)

	// Keys alone are something to choose.
	l = &List{Invalid: errInvalidThing, Keys: []string{"q"}}
	c, err := l.Choose(&bytes.Buffer{}, strings.NewReader("q\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Key: "q"}, c)
}

// With First, the row Enter picks is shown by the number it is shown with.
func TestChooseDefaultWithFirst(t *testing.T) {
	l := pages()
	l.Default = 2
	var out bytes.Buffer
	c, err := l.Choose(&out, strings.NewReader("\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Row: 1}, c)
	assert.True(t, strings.HasSuffix(out.String(), "\n> [5] "), "%q", out.String())
}

// With ByName a row named like a key would make one answer mean both, so
// such a List is a programming error; without ByName the key answers, and a
// ByName row named otherwise is picked by its name beside the keys.
func TestChooseRefusesARowNamedLikeAKey(t *testing.T) {
	l := &List{Header: []string{"NAME"}, Invalid: errInvalidThing, Keys: []string{"n"}, ByName: true}
	l.AddRow(false, "first")
	l.AddRow(false, "n")
	assert.PanicsWithValue(t, `picker: a ByName row is named "n", one of its Keys`, func() {
		_, _ = l.Choose(&bytes.Buffer{}, strings.NewReader("n\n"))
	})

	l.ByName = false
	c, err := l.Choose(&bytes.Buffer{}, strings.NewReader("n\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Key: "n"}, c)

	l = &List{Header: []string{"NAME"}, Invalid: errInvalidThing, Keys: []string{"n"}, ByName: true}
	l.AddRow(false, "first")
	for answer, want := range map[string]Choice{"first\n": {Row: 0}, "n\n": {Key: "n"}} {
		c, err := l.Choose(&bytes.Buffer{}, strings.NewReader(answer))
		require.NoError(t, err)
		assert.Equal(t, want, c, "answer %q", answer)
	}
}

// First numbers the rows from where the page starts, and they are answered by
// the numbers shown; Keys answer as themselves; Hint is shown above the
// prompt each time it is asked.
func TestChooseKeysAndFirst(t *testing.T) {
	var out bytes.Buffer
	c, err := pages().Choose(&out, strings.NewReader("5\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Row: 1}, c)
	lines := strings.Split(out.String(), "\n")
	assert.Equal(t, []string{"4", "four"}, strings.Fields(lines[1]))
	assert.True(t, strings.HasSuffix(out.String(), "\n\np. previous n. next\n> "), "%q", out.String())

	c, err = pages().Choose(&bytes.Buffer{}, strings.NewReader("n\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Key: "n"}, c)

	for _, answer := range []string{"1\n", "3\n", "6\n", "N\n", "q\n", " n\n"} {
		_, err := pages().Choose(&bytes.Buffer{}, strings.NewReader(answer))
		assert.ErrorIs(t, err, errInvalidThing, "answer %q", answer)
	}

	out.Reset()
	c, err = pages().Choose(&out, strings.NewReader("x\np\n"))
	require.NoError(t, err)
	assert.Equal(t, Choice{Key: "p"}, c, "a key after a wrong answer")
	assert.Equal(t, 1, strings.Count(out.String(), "Not one of the choices.\np. previous n. next\n> "), "%q", out.String())
}

// InvalidAnswer names the last answer given, not the nothing that ended
// the input after it.
func TestPickInvalidAnswerNamesTheLastAnswerGiven(t *testing.T) {
	for in, want := range map[string]string{"9\n": "9", "9\nprd": "prd", "9\n\n": ""} {
		l := things()
		l.InvalidAnswer = func(answer string) error { return fmt.Errorf("%w: %q", errInvalidThing, answer) }
		_, err := l.Pick(&bytes.Buffer{}, strings.NewReader(in))
		require.ErrorIs(t, err, errInvalidThing)
		assert.EqualError(t, err, fmt.Sprintf("invalid thing selection: %q", want), "input %q", in)
	}
}

// A pick and the confirmation after it, answered from one pipe as a script
// answers them: the picker reads stdin through the reader the confirmation
// reads, so the "y" it read ahead with its own answer is still there.
func TestPickThenConfirmFromOnePipe(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("2\ny\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin; r.Close() })

	var out bytes.Buffer
	i, err := things().Pick(&out, os.Stdin)
	require.NoError(t, err)
	assert.Equal(t, 1, i)
	ok, err := input.Confirm("Delete it?")
	require.NoError(t, err)
	assert.True(t, ok, "the confirmation lost the answer the picker read ahead")
}
