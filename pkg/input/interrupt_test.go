package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// stdinNeverAnswering makes os.Stdin a pipe nothing is ever written to, open
// until the subtest ends.
func (s *Suite) stdinNeverAnswering() {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	saved := os.Stdin
	os.Stdin = r
	s.T().Cleanup(func() {
		os.Stdin = saved
		w.Close()
		r.Close()
	})
}

// onceWaiting returns a channel closed when a read is blocked waiting for its
// line, until the subtest ends.
func (s *Suite) onceWaiting() <-chan struct{} {
	waiting := make(chan struct{})
	saved := waitingForALine
	waitingForALine = func() { close(waiting) }
	s.T().Cleanup(func() { waitingForALine = saved })
	return waiting
}

// Canceling the run while a question waits for a line ends it at once with
// ErrInterrupted, for every line question: the first Ctrl-C at a (y/n) used
// to echo ^C and go on waiting.
func (s *Suite) TestALineQuestionEndsOnInterrupt() {
	asks := map[string]func() error{
		"ConfirmOrAbort": func() error { return ConfirmOrAbort("Delete it?", AnsweredBy("--yes")) },
		"Confirm":        func() error { _, err := Confirm("Delete it?"); return err },
		"Text":           func() error { _, err := Text("Name: "); return err },
	}
	for name, ask := range asks {
		s.Run(name, func() {
			s.stdinNeverAnswering()
			waiting := s.onceWaiting()
			ctx, cancel := context.WithCancel(context.Background())
			s.T().Cleanup(SetInterrupt(ctx))

			got := make(chan error, 1)
			go func() { got <- ask() }()
			select {
			case <-waiting:
			case <-time.After(2 * time.Second):
				s.FailNow("the question never started reading")
			}
			cancel()

			select {
			case err := <-got:
				s.ErrorIs(err, ErrInterrupted)
			case <-time.After(2 * time.Second):
				s.Fail("the question went on waiting after the interrupt")
			}
		})
	}
}

// A question reached after the run was interrupted is refused before it is
// asked or reads anything: the answer waiting on stdin is still there.
func (s *Suite) TestAQuestionAfterTheInterruptReadsNothing() {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.WriteString("y\n")
	s.Require().NoError(err)
	s.Require().NoError(w.Close())
	saved := os.Stdin
	os.Stdin = r
	s.T().Cleanup(func() { os.Stdin = saved; r.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.T().Cleanup(SetInterrupt(ctx))

	s.ErrorIs(ConfirmOrAbort("Delete it?", AnsweredBy("--yes")), ErrInterrupted)
	s.ErrorIs(MayAsk("Delete it?"), ErrInterrupted)
	_, err = Password("Password: ")
	s.ErrorIs(err, ErrInterrupted)
	left, err := io.ReadAll(r)
	s.Require().NoError(err)
	s.Equal("y\n", string(left), "the answer was consumed")
}

// A read that ends (a console read returns nothing on Ctrl-C on Windows)
// while the run is being interrupted is the interrupt, not ended input.
func (s *Suite) TestAnEndedReadDuringTheInterruptIsTheInterrupt() {
	ctx, cancel := context.WithCancel(context.Background())
	s.T().Cleanup(SetInterrupt(ctx))
	saved := waitingForALine
	waitingForALine = cancel // the run is interrupted as the read ends
	s.T().Cleanup(func() { waitingForALine = saved })

	r, w := io.Pipe()
	s.Require().NoError(w.Close())
	_, err := ReadLine(bufio.NewReader(r), io.Discard)
	s.ErrorIs(err, ErrInterrupted)
}

// The Windows ordering: the console read returns EOF for the Ctrl-C first,
// and the signal cancels the run a moment later. Within endedReadGrace that
// EOF is the interrupt; with no grace (Unix), it is ended input, at once.
func (s *Suite) TestAnEOFJustBeforeTheInterruptIsTheInterrupt() {
	saved := endedReadGrace
	s.T().Cleanup(func() { endedReadGrace = saved })

	// interruptLater cancels the run 20ms after the read ends, or never.
	read := func(interruptLater bool) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer SetInterrupt(ctx)()
		defer cancel()
		r, w := io.Pipe()
		s.Require().NoError(w.Close()) // the read ends with io.EOF at once
		if interruptLater {
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
		}
		_, err := ReadLine(bufio.NewReader(r), io.Discard)
		return err
	}

	endedReadGrace = func() time.Duration { return time.Second }
	s.ErrorIs(read(true), ErrInterrupted)

	// With no grace, or no interrupt within it, ended input is ended input.
	s.ErrorIs(read(false), io.EOF)
	endedReadGrace = func() time.Duration { return 0 }
	s.ErrorIs(read(false), io.EOF)
}

// The bufio.Reader an interrupted read was left reading is dropped, so a
// question in a later run does not share it.
func (s *Suite) TestAnInterruptedReadLetsGoOfTheSharedReader() {
	s.stdinNeverAnswering()
	waiting := s.onceWaiting()
	ctx, cancel := context.WithCancel(context.Background())
	restore := SetInterrupt(ctx)
	r := stdin()

	got := make(chan error, 1)
	go func() { _, err := ReadLine(r, io.Discard); got <- err }()
	<-waiting
	cancel()
	s.ErrorIs(<-got, ErrInterrupted)
	restore()

	s.NotSame(r, stdin(), "the next question shares the reader the old read still holds")
}

// A refusal and an interrupt are unanswered questions; an ended read or a no
// is an answer of sorts.
func (s *Suite) TestUnanswered() {
	s.True(Unanswered(Required(io.EOF)))
	s.True(Unanswered(fmt.Errorf("asking: %w", ErrInterrupted)))
	s.False(Unanswered(io.EOF))
	s.False(Unanswered(ErrAborted))
	s.False(Unanswered(nil))
}

// With no run installed, or one never interrupted, a line is read as before.
func (s *Suite) TestReadLineWithoutAnInterrupt() {
	s.stdinHolding("yes\n")
	s.NoError(ConfirmOrAbort("Delete it?"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.T().Cleanup(SetInterrupt(ctx))
	s.stdinHolding("n\n")
	s.ErrorIs(ConfirmOrAbort("Delete it?"), ErrAborted)
}
