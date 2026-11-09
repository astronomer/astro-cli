package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"time"
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
// the interrupt is refused before it reads (MayAsk). The binary exits right
// after. In a process that goes on to another run (an embedder, a test), the
// process-wide bufio.Reader it holds is dropped (forget), so the next question
// does not share a bufio.Reader with it; but the goroutine still reads the
// same file, and the next line typed goes to whichever read gets it first.
// Nothing short of closing stdin stops a blocked read portably.
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
		if g.err != nil && interruptedWithin(done, endedReadGrace) {
			return "", endLine(w)
		}
		return g.line, g.err
	case <-done:
		forget(r)
		return "", endLine(w)
	}
}

// endedReadGrace is how long a read that ended in an error waits for the
// interrupt that may have ended it, before it is taken as ended input.
//
// On Windows a console read that gets Ctrl-C returns no bytes, which Go
// reports as io.EOF, and the signal cancels the run separately, from the
// runtime's console control handler thread, usually just after the read has
// returned. Read as it arrives, that EOF is "stdin ended" (pass --yes) or a
// no, not the interrupt it is. The wait covers that gap. On Unix a terminal
// turns Ctrl-C into the signal alone and the read goes on waiting, so an
// ended read there is ended input, read at once: a script whose stdin is
// /dev/null is not slowed.
var endedReadGrace = func() time.Duration {
	if runtime.GOOS == "windows" {
		return windowsConsoleInterruptGap
	}
	return 0
}()

// windowsConsoleInterruptGap bounds the gap between a console read ending on
// Ctrl-C and the signal reaching the run: the console control handler runs on
// a thread of its own, and is scheduled within a few milliseconds; 100ms is
// well past that and still too short for a person to notice.
const windowsConsoleInterruptGap = 100 * time.Millisecond

// interruptedWithin reports whether done is closed now or within wait.
func interruptedWithin(done <-chan struct{}, wait time.Duration) bool {
	if wait <= 0 {
		return isInterrupted()
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// endLine ends the line a question was asked on, on w, and returns
// ErrInterrupted.
func endLine(w io.Writer) error {
	fmt.Fprintln(w)
	return ErrInterrupted
}
