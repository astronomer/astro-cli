package input

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
)

// The guard is the one place a run says it may not be asked anything.
//
// A process-level setting rather than a context value, because the questions
// are asked from deep in the v1 platform packages, through call chains that
// carry no context — threading one to each of them would be a signature change
// on every function between a command and its prompt, for a fact that is the
// same for the whole run. A run is one command, so one setting is the right
// grain. The root sets it once (cmd/cliout.Execute) and the primitives here
// consult it, so no call site has to remember to.
//
// A function rather than a bool so the root can set it before cobra has parsed
// the command's flags and still answer from the parsed value: it is only
// called when a question is about to be asked, by which time they are parsed.
var guard atomic.Pointer[func() string]

// SetGuard installs refuse, which every prompt consults before it asks
// anything: a non-empty answer refuses the prompt, and is the reason the
// refusal gives ("with --output json it cannot"). It returns a function that
// puts back the guard it replaced. A nil refuse removes the guard.
func SetGuard(refuse func() string) (restore func()) {
	var next *func() string
	if refuse != nil {
		next = &refuse
	}
	prev := guard.Swap(next)
	return func() { guard.Store(prev) }
}

// refusal is the guard's answer now: "" when this run may ask.
func refusal() string {
	g := guard.Load()
	if g == nil {
		return ""
	}
	return (*g)()
}

// Option describes a prompt beyond its text, for the refusal to name.
type Option func(*RequiredError)

// AnsweredBy names the flag that supplies the answer instead ("--force",
// "--deployment"), so a refusal can say exactly what to pass.
func AnsweredBy(flag string) Option {
	return func(e *RequiredError) { e.Flag = flag }
}

// About says what is being asked, for a prompt whose own text says nothing —
// the bare "> " under a table of choices.
func About(what string) Option {
	return func(e *RequiredError) { e.About = what }
}

// MayAsk returns nil when this run may ask prompt, and otherwise the
// *RequiredError a prompt refuses with. A caller that reads its answer from a
// reader of its own (a table picker, a y/N on stderr) calls it first, so it
// refuses the same way the primitives here do.
func MayAsk(prompt string, opts ...Option) error {
	reason := refusal()
	if reason == "" {
		return nil
	}
	e := &RequiredError{Prompt: prompt, Reason: reason}
	for _, o := range opts {
		o(e)
	}
	return e
}

// RequiredError is a question this run needed answered and could not ask. The
// answer has to come with the invocation instead: a flag, an argument.
type RequiredError struct {
	// Prompt is the question as it would have been asked.
	Prompt string
	// About says what was asked when Prompt does not (see About).
	About string
	// Flag supplies the answer, when the asker knows which one does.
	Flag string
	// Reason is why this run could not ask.
	Reason string

	// err is an asker's own refusal, marked by Required; when set it is the
	// whole message.
	err error
}

func (e *RequiredError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	asked := "for an answer"
	switch prompt := strings.TrimRight(strings.TrimSpace(e.Prompt), ": >"); {
	case e.About != "":
		asked = "for " + e.About
	case prompt != "":
		asked = fmt.Sprintf("%q", prompt)
	}
	answer := "pass the answer as a flag"
	if e.Flag != "" {
		answer = "pass " + e.Flag
	}
	reason := e.Reason
	if reason == "" {
		reason = "this run cannot ask"
	}
	return fmt.Sprintf("this command needs to ask %s; %s — %s", asked, reason, answer)
}

func (e *RequiredError) Unwrap() error { return e.err }

// Required marks err, an asker's own refusal to ask, as a *RequiredError, so a
// question refused in its own words is recognized as one refused here. The
// message is err's, unchanged.
func Required(err error) error {
	if err == nil {
		return nil
	}
	return &RequiredError{err: err}
}

// IsRequired reports whether err is a question this run could not ask.
func IsRequired(err error) bool {
	var r *RequiredError
	return errors.As(err, &r)
}
