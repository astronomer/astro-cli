package input

import (
	"context"
	"io"
	"os"
	"time"
)

// stdinNeverAnswering makes os.Stdin a pipe nothing is ever written to, open
// until the subtest ends, and returns its write end.
func (s *Suite) stdinNeverAnswering() *os.File {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	saved := os.Stdin
	os.Stdin = r
	s.T().Cleanup(func() {
		os.Stdin = saved
		w.Close()
		r.Close()
	})
	return w
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
			ctx, cancel := context.WithCancel(context.Background())
			s.T().Cleanup(SetInterrupt(ctx))

			got := make(chan error, 1)
			go func() { got <- ask() }()
			time.Sleep(50 * time.Millisecond) // blocked on the read by now
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

// A question asked after the run was interrupted reads nothing: the answer
// waiting on stdin is still there.
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
	left, err := io.ReadAll(r)
	s.Require().NoError(err)
	s.Equal("y\n", string(left), "the answer was consumed")
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
