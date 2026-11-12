package input

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// askConfirmation drives a Confirmation with the given stdin (what a person
// types at a terminal) and reports what it returned and wrote.
func askConfirmation(stdin string) (string, error) {
	var out bytes.Buffer
	err := (&Confirmation{
		Question: "Wipe it?",
		Choices:  " [y/N] ",
		In:       strings.NewReader(stdin),
		Out:      &out,
		Options:  []Option{AnsweredBy("--yes")},
	}).Ask()
	return out.String(), err
}

// Every way a confirmation ends leaves its output with whole lines: the
// question is written first, the terminal echoes the answer itself, and input
// that ended (Ctrl-D) has its line closed before the refusal. Input that is
// nothing but escapes is no answer, and the answer is decided on its Plain
// text. A run with no terminal is refused before any of this
// (TestConfirmationRefusedBeforeAnythingIsWritten).
func TestConfirmationLeavesWholeLines(t *testing.T) {
	const (
		q       = "Wipe it? [y/N] "
		refused = "confirmation needed but input ended with no answer; pass --yes to proceed"
	)
	for name, tc := range map[string]struct {
		stdin   string
		out     string
		refusal string // the input_required refusal naming --yes
		err     error  // otherwise; nil for a yes
	}{
		"yes":                           {"y\n", q, "", nil},
		"no":                            {"n\n", q, "", ErrAborted},
		"input ended":                   {"", q + "\n", refused, nil},
		"yes then ended":                {"y", q + "\n", "", nil},
		"an empty line (a no)":          {"\n", q, "", ErrAborted},
		"only escapes and no newline":   {"\x1b[0m", q + "\n", refused, nil},
		"only invisible and no newline": {"\x1b[0m\u200b", q + "\n", refused, nil},
		"a joiner keeps y-es from yes":  {"y\u200des\n", q, "", ErrAborted},
		"a styled yes is a yes":         {"y\x1b[0m\x07\n", q, "", nil},
		"YES in any case":               {"YES\r\n", q, "", nil},
		"maybe is a no":                 {"maybe\n", q, "", ErrAborted},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := askConfirmation(tc.stdin)
			assert.Equal(t, tc.out, out)
			switch {
			case tc.refusal != "":
				require.Error(t, err)
				assert.True(t, IsRequired(err), "%v", err)
				assert.EqualError(t, err, tc.refusal)
			case tc.err == nil:
				assert.NoError(t, err)
			default:
				assert.ErrorIs(t, err, tc.err)
			}
		})
	}
}

// A run that may not ask is refused before the question is written or the
// answer read, naming the flag, whatever the reason.
func TestConfirmationRefusedBeforeAnythingIsWritten(t *testing.T) {
	for _, reason := range []string{"with --output json it cannot", "with CI set it cannot", "with --non-interactive it cannot"} {
		t.Run(reason, func(t *testing.T) {
			defer SetGuard(func() string { return reason })()
			var out bytes.Buffer
			in := strings.NewReader("y\n")
			err := (&Confirmation{Question: "Wipe it?", In: in, Out: &out, Options: []Option{AnsweredBy("--yes")}}).Ask()
			require.Error(t, err)
			assert.True(t, IsRequired(err), "%v", err)
			assert.Contains(t, err.Error(), reason)
			assert.Contains(t, err.Error(), "pass --yes")
			assert.Empty(t, out.String(), "nothing written")
			assert.Equal(t, 2, in.Len(), "nothing read")
		})
	}

	t.Run("ConfirmOrAbort too", func(t *testing.T) {
		defer SetGuard(func() string { return "with CI set it cannot" })()
		err := ConfirmOrAbort("Wipe it?", AnsweredBy("--yes"))
		assert.True(t, IsRequired(err), "%v", err)
	})
}

// blockingReader is a stdin nobody answers: its read interrupts the run, as
// Ctrl-C would while the question waits, then blocks until the test ends.
type blockingReader struct {
	cancel  context.CancelFunc
	release chan struct{}
}

func (b blockingReader) Read([]byte) (int, error) {
	b.cancel()
	<-b.release
	return 0, context.Canceled
}

// Ctrl-C ends a question at once, with ErrInterrupted, and closes the
// question's line first, a terminal on stdin or not.
func TestConfirmationInterruptedEndsTheQuestionsLine(t *testing.T) {
	for name := range map[string]bool{"terminal": true} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			t.Cleanup(SetInterrupt(ctx))
			in := blockingReader{cancel: cancel, release: make(chan struct{})}
			t.Cleanup(func() { close(in.release) })

			var out bytes.Buffer
			err := (&Confirmation{Question: "Wipe it?", Choices: " [y/N] ", In: in, Out: &out}).Ask()
			assert.ErrorIs(t, err, ErrInterrupted)
			assert.Equal(t, "Wipe it? [y/N] \n", out.String())
		})
	}
}

// partialThenFails reads "y" and then fails, as a broken pipe or a console that
// went away does.
type partialThenFails struct{ done bool }

func (p *partialThenFails) Read(b []byte) (int, error) {
	if p.done {
		return 0, errors.New("read failed")
	}
	p.done = true
	return copy(b, "y"), nil
}

// A read that fails after some text is not a yes: only a line that ended at
// the end of input counts as an unterminated answer.
func TestConfirmationReadFailureWithPartialTextIsNotAYes(t *testing.T) {
	for name := range map[string]bool{"terminal": true} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := (&Confirmation{Question: "Wipe it?", In: &partialThenFails{}, Out: &out}).Ask()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "read failed")
			assert.NotErrorIs(t, err, ErrAborted)
			assert.False(t, IsRequired(err), "%v", err)
		})
	}
}

// Ended input is the same structured refusal as Text, Confirm and a picker
// return: the question, the flag and the reason are there to read, whatever
// the message says.
func TestConfirmationEndedIsAStructuredRequiredError(t *testing.T) {
	_, err := askConfirmation("")
	var req *RequiredError
	require.ErrorAs(t, err, &req)
	assert.Equal(t, "Wipe it?", req.Prompt)
	assert.Equal(t, "--yes", req.Flag)
	assert.Equal(t, EndedReason, req.Reason)
}

// A question that names no flag still refuses ended input, saying to pass the
// answer as a flag.
func TestConfirmationEndedWithNoFlagNamed(t *testing.T) {
	err := (&Confirmation{Question: "Wipe it?", In: strings.NewReader(""), Out: &bytes.Buffer{}}).Ask()
	require.Error(t, err)
	assert.True(t, IsRequired(err))
	assert.Contains(t, err.Error(), "pass the answer as a flag")
}
