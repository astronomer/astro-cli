package input

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// Confirmation is a yes/no question asked of a reader and a writer of the
// caller's, the one implementation behind every confirmation in the CLI
// (ConfirmOrAbort asks it on stdin and stderr, astro local on its own streams).
type Confirmation struct {
	// Question is what is asked, without the choices.
	Question string
	// Choices follow the question on its line: " (y/n) " by default,
	// " [y/N] " for a question that defaults to no.
	Choices string
	// In is where the answer is read from, Out where the question is written.
	In  io.Reader
	Out io.Writer
	// Terminal is whether In is a terminal: one echoes what is typed, a pipe
	// does not, and it words a refusal ("input ended" or "stdin is not
	// interactive").
	Terminal bool
	// Options name the flag that answers the question (AnsweredBy) and what is
	// asked (About), for a run that may not ask, and for ended input.
	Options []Option
}

// Ask asks the question and returns nil for a yes (y or yes, in any case),
// ErrAborted for any other answer, and ErrInterrupted at Ctrl-C.
//
// A run that may not ask (see Refusal) is refused with a *RequiredError before
// anything is written or read. Otherwise the question is always written, on a
// terminal or not: a stdin that is not one often still has a person behind it
// (ssh or exec without a tty, mintty), or a tool that waits for the question
// before it answers. Every way the read ends leaves the question's line whole:
// Ctrl-C and input that ended close it before the error, and an answer from a
// pipe, which nothing echoes, is written after the question as plain text
// (Plain), so a log reads "...? [y/N] n". The answer is decided on that same
// text, and input that is nothing but escapes is no answer, not a no.
//
// Input that ended with no answer is not a no: nobody answered, so it is a
// *RequiredError naming the flag that answers the question.
func (q *Confirmation) Ask() error {
	if err := MayAsk(q.Question, q.Options...); err != nil {
		return err
	}
	choices := q.Choices
	if choices == "" {
		choices = " (y/n) "
	}
	fmt.Fprint(q.Out, q.Question+choices) //nolint:errcheck // a failed write to the terminal still reads the answer
	line, err := ReadLine(Reader(q.In), q.Out)
	if errors.Is(err, ErrInterrupted) {
		return err // ReadLine ended the line
	}
	answer := Plain(line)
	// An empty line is an answer: a no. A read that failed is none, unless
	// it brought text with it.
	answered := err == nil || (errors.Is(err, io.EOF) && answer != "")
	switch {
	case answered && !q.Terminal:
		// A terminal echoed what was typed; a pipe did not.
		fmt.Fprintln(q.Out, answer)
	case err != nil:
		// Input that ended left the line open: end it, so what follows
		// starts on a fresh one.
		fmt.Fprintln(q.Out)
	}
	if !answered {
		switch {
		case errors.Is(err, io.EOF):
			return q.ended()
		case !q.Terminal:
			return fmt.Errorf("reading the answer to %s: %w", Quote(q.Question), err)
		}
		return err
	}
	switch strings.ToLower(answer) {
	case answerY, answerYes:
		return nil
	default:
		return ErrAborted
	}
}

// ended is the refusal for input that ended with no answer.
func (q *Confirmation) ended() error {
	named := &RequiredError{Prompt: q.Question, Reason: EndedReason}
	for _, o := range q.Options {
		o(named)
	}
	why := "stdin is not interactive"
	if q.Terminal {
		why = "input ended with no answer"
	}
	answer := "pass the answer as a flag"
	if named.Flag != "" {
		answer = "pass " + named.Flag
	}
	// The same structured refusal Text, Confirm and the pickers return (the
	// question, the flag, the reason), in this confirmation's own words.
	named.err = fmt.Errorf("confirmation needed but %s; %s to proceed", why, answer)
	return named
}

// The answers that are a yes, in any case.
const (
	answerY   = "y"
	answerYes = "yes"
)
