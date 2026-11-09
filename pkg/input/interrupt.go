package input

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync/atomic"
)

// run is the context of the run questions are asked in, or nil.
//
// The first Ctrl-C of a run cancels this context (the binary's
// signal.NotifyContext) rather than killing the process, so a command can
// unwind what it started. A question blocked reading a line from stdin never
// looked at it: the terminal echoed ^C, the read went on waiting, and only a
// second Ctrl-C, after the handler was restored, or an Enter got the person
// out. Process-level for the reason the guard is (see guard): the questions
// are asked from call chains that carry no context.
var run atomic.Pointer[context.Context]

// SetInterrupt installs ctx as the run's, whose cancellation ends a question
// waiting for a line (ReadLine) with ErrInterrupted. The root sets it for the
// run (cmd/cliout.Execute), next to the guard. It returns a function that puts
// back the context it replaced. A nil ctx removes it.
func SetInterrupt(ctx context.Context) (restore func()) {
	var next *context.Context
	if ctx != nil {
		next = &ctx
	}
	prev := run.Swap(next)
	return func() { run.Store(prev) }
}

// interrupted is the run's done channel, or nil when no run is installed.
func interrupted() <-chan struct{} {
	ctx := run.Load()
	if ctx == nil {
		return nil
	}
	return (*ctx).Done()
}

// ReadLine reads one answer from r, as r.ReadString('\n') does, unless the run
// is interrupted first: then it returns ErrInterrupted at once, having ended
// the line the question was asked on so the error starts on a fresh one.
// Every question that reads a line from stdin reads it here.
//
// The read waits in a goroutine of its own, so the interrupt does not have to
// wait for it. When the interrupt wins, that goroutine stays blocked on stdin
// until a line comes or the process exits. It cannot take an answer meant for
// a later question: an interrupted run is ending, and every question after
// the interrupt sees the run already done and returns before it touches r.
func ReadLine(r *bufio.Reader) (string, error) {
	done := interrupted()
	if done == nil {
		return r.ReadString('\n')
	}
	select {
	case <-done:
		return "", lineInterrupted()
	default:
	}
	type read struct {
		line string
		err  error
	}
	got := make(chan read, 1)
	go func() {
		line, err := r.ReadString('\n')
		got <- read{line, err}
	}()
	select {
	case g := <-got:
		return g.line, g.err
	case <-done:
		return "", lineInterrupted()
	}
}

// lineInterrupted ends the question's line on stderr, where every question is
// asked, and returns ErrInterrupted.
func lineInterrupted() error {
	fmt.Fprintln(os.Stderr)
	return ErrInterrupted
}
