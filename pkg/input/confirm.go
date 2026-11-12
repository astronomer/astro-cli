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
	// Options name the flag that answers the question (AnsweredBy) and what is
	// asked (About), for a run that may not ask, and for ended input.
	Options []Option
}

// Ask asks the question and returns nil for a yes (y or yes, in any case),
// ErrAborted for any other answer, and ErrInterrupted at Ctrl-C.
//
// A run that may not ask (see Refusal: no terminal on stdin or stderr, CI,
// json, --non-interactive) is refused with a *RequiredError before anything is
// written or read. Otherwise a person is at a terminal, which echoes what is
// typed, so the question is written and the answer read from the line. Ctrl-C
// and input that ended (Ctrl-D) close the question's line before the error. The
// answer is decided on its Plain text, and input that is nothing but escapes is
// no answer, not a no.
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
	if err != nil {
		// Input that ended left the line open: end it, so what follows
		// starts on a fresh one.
		fmt.Fprintln(q.Out)
	}
	if !answered {
		if errors.Is(err, io.EOF) {
			return q.ended()
		}
		return fmt.Errorf("reading the answer to %s: %w", Quote(q.Question), err)
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
	named := endedRequired(q.Question, q.Options).(*RequiredError) //nolint:forcetypeassert // endedRequired returns one
	answer := "pass the answer as a flag"
	if named.Flag != "" {
		answer = "pass " + named.Flag
	}
	// The same structured refusal Text, Confirm and the pickers return (the
	// question, the flag, the reason), in this confirmation's own words.
	named.err = fmt.Errorf("confirmation needed but input ended with no answer; %s to proceed", answer)
	return named
}

// The answers that are a yes, in any case.
const (
	answerY   = "y"
	answerYes = "yes"
)
