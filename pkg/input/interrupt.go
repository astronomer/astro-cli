package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync/atomic"
)

// runDone is the done channel of the run questions are asked in, or nil.
//
// The first Ctrl-C of a run cancels the run's context (the binary's
// signal.NotifyContext) rather than killing the process, so a command can
// unwind what it started. A question blocked reading a line from stdin never
// looked at it: the terminal echoed ^C, the read went on waiting, and only a
// second Ctrl-C, after the handler was restored, or an Enter got the person
// out. Process-level for the reason the guard is (see guard): the questions
// are asked from call chains that carry no context.
var runDone atomic.Pointer[<-chan struct{}]

// SetInterrupt installs ctx as the run's, whose cancellation ends a question:
// one about to be asked is refused (MayAsk), and one waiting for a line
// (ReadLine) returns ErrInterrupted. The root sets it for the run
// (cmd/cliout.Execute), next to the guard. It returns a function that puts
// back what it replaced. A nil ctx, or one that is never canceled, installs
// nothing.
func SetInterrupt(ctx context.Context) (restore func()) {
	var next *<-chan struct{}
	if ctx != nil {
		if done := ctx.Done(); done != nil {
			next = &done
		}
	}
	prev := runDone.Swap(next)
	return func() { runDone.Store(prev) }
}

// interrupted is the run's done channel, or nil when no run is installed.
func interrupted() <-chan struct{} {
	if done := runDone.Load(); done != nil {
		return *done
	}
	return nil
}

// isInterrupted reports whether the run has been interrupted.
func isInterrupted() bool {
	select {
	case <-interrupted():
		return true
	default:
		return false
	}
}

// waitingForALine is called once a read is blocked waiting for its line; a
// test sets it to know the interrupt arrives during the read, not before it.
var waitingForALine = func() {}

// ReadLine reads one answer from r, as r.ReadString('\n') does, unless the run
// is interrupted first: then it returns ErrInterrupted at once, having ended
// the prompt's line on w, the writer the prompt was written to, so the error
// starts on a fresh one. Every question pkg/input, pkg/picker and astro
// local's [y/N] ask reads its line here.
//
// The read waits in a goroutine of its own, so the interrupt does not have to
// wait for it. When the interrupt wins, that goroutine stays blocked on stdin
// until a line comes or the process exits. It cannot take an answer meant for
// a later question of the run: the run is ending, and every question after
// the interrupt is refused before it reads (MayAsk). The process-wide reader
// it holds is also dropped (forget), so a question in a later run in the same
// process (an embedder, a test) starts a reader of its own rather than share
// one another goroutine is reading.
func ReadLine(r *bufio.Reader, w io.Writer) (string, error) {
	done := interrupted()
	if done == nil {
		return r.ReadString('\n')
	}
	if isInterrupted() {
		return "", endLine(w)
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
	waitingForALine()
	select {
	case g := <-got:
		// A Ctrl-C can end the read too (a console read returns nothing
		// on Windows) while the signal is still canceling the run. An
		// ended read the run was interrupted during is the interrupt.
		if g.err != nil && isInterrupted() {
			return "", endLine(w)
		}
		return g.line, g.err
	case <-done:
		forget(r)
		return "", endLine(w)
	}
}

// endLine ends the line a question was asked on, on w, and returns
// ErrInterrupted.
func endLine(w io.Writer) error {
	fmt.Fprintln(w)
	return ErrInterrupted
}
