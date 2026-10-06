package picker

import (
	"bytes"
	"errors"
	"fmt"
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
