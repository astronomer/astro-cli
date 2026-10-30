package input

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

// refuseJSON is the guard the root installs for a run under --output json.
func refuseJSON() string { return "with --output json it cannot" }

// stdinWith replaces os.Stdin with a pipe holding answer, and reports whether
// anything read it: a refused prompt must leave its answer unread.
func (s *Suite) stdinWith(answer string) (unread func() bool) {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.WriteString(answer)
	s.Require().NoError(err)
	s.Require().NoError(w.Close())
	stdin := os.Stdin
	os.Stdin = r
	s.T().Cleanup(func() { os.Stdin = stdin; r.Close() })
	return func() bool {
		buf := make([]byte, len(answer))
		n, _ := r.Read(buf)
		return n == len(answer)
	}
}

// Under the guard, every primitive refuses without reading stdin, and the
// refusal is a *RequiredError — never a yes, a no, or an empty answer.
func (s *Suite) TestGuardRefusesEveryPrimitive() {
	defer SetGuard(refuseJSON)()

	prompts := map[string]func() error{
		"Text": func() error {
			got, err := Text("Team name: ")
			s.Empty(got)
			return err
		},
		"Confirm": func() error {
			got, err := Confirm("Are you sure?")
			s.False(got)
			return err
		},
		"Password": func() error {
			got, err := Password("Password: ")
			s.Empty(got)
			return err
		},
		"MayAsk": func() error { return MayAsk("> ") },
	}
	for name, ask := range prompts {
		s.Run(name, func() {
			unread := s.stdinWith("y\n")
			err := ask()
			s.True(IsRequired(err), "want a *RequiredError, got %v", err)
			s.True(unread(), "a refused prompt read stdin")
		})
	}
}

// With no guard, or one that answers "", the primitives ask as they always
// have.
func (s *Suite) TestNoGuardAsks() {
	defer SetGuard(func() string { return "" })()
	s.stdinWith("y\n")
	got, err := Confirm("Are you sure?")
	s.NoError(err)
	s.True(got)

	defer SetGuard(nil)()
	s.stdinWith("prod\n")
	text, err := Text("Name: ")
	s.NoError(err)
	s.Equal("prod", text)
}

// The restore puts back the guard it replaced, so nested runs unwind.
func (s *Suite) TestSetGuardRestores() {
	outer := SetGuard(refuseJSON)
	inner := SetGuard(nil)
	s.NoError(MayAsk("q"))
	inner()
	s.Error(MayAsk("q"))
	outer()
	s.NoError(MayAsk("q"))
}

// The refusal names what was asked and how to answer it instead.
func (s *Suite) TestRequiredErrorMessage() {
	defer SetGuard(refuseJSON)()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"the question and the flag",
			MayAsk("Are you sure you want to delete prod?", AnsweredBy("--force")),
			`this command needs to ask "Are you sure you want to delete prod?"; with --output json it cannot — pass --force`,
		},
		{
			"a bare picker prompt says what it picks",
			MayAsk("\n> ", About("a workspace"), AnsweredBy("--workspace-id")),
			"this command needs to ask for a workspace; with --output json it cannot — pass --workspace-id",
		},
		{
			"a bare prompt with nothing known is still a sentence",
			MayAsk("\n> "),
			"this command needs to ask for an answer; with --output json it cannot — pass the answer as a flag",
		},
		{
			"trailing prompt punctuation is dropped",
			MayAsk("Enter a name for the new token: "),
			`this command needs to ask "Enter a name for the new token"; with --output json it cannot — pass the answer as a flag`,
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() { s.EqualError(tc.err, tc.want) })
	}
}

// Required keeps an asker's own words and wrap chain, and still reads as a
// refused question through further wrapping.
func (s *Suite) TestRequiredWrapsAnAskersOwnRefusal() {
	base := errors.New("pass --deployment")
	err := fmt.Errorf("deploying: %w", Required(base))
	s.True(IsRequired(err))
	s.ErrorIs(err, base)
	s.EqualError(err, "deploying: pass --deployment")
	s.NoError(Required(nil))
	s.False(IsRequired(base))
}

// Two questions answered from one pipe each get their own line: the first
// read takes the whole pipe into the reader, and the second finds its answer
// there instead of an empty stdin.
func (s *Suite) TestQuestionsAnsweredFromOnePipe() {
	defer SetGuard(nil)()
	s.stdinWith("staging\ny\n")
	got, err := Text("Which one? ")
	s.Require().NoError(err)
	s.Equal("staging", got)
	ok, err := Confirm("Sure?")
	s.Require().NoError(err)
	s.True(ok)
}

// sliceReader is a reader whose type cannot be compared, and wrapper one that
// passes as comparable until its field holds one: comparing two wrappers
// then panics.
type (
	sliceReader []byte
	wrapper     struct{ io.Reader }
)

func (r sliceReader) Read(p []byte) (int, error) { return copy(p, r), io.EOF }

// Only a file is shared. Any other source, a wrapper around one that cannot
// be compared included, is read through a reader of its own, and asking for
// one never compares it with the last.
func (s *Suite) TestReaderSharesOnlyFiles() {
	first := Reader(wrapper{sliceReader("a\n")})
	var second *bufio.Reader
	s.NotPanics(func() { second = Reader(wrapper{sliceReader("b\n")}) })
	s.NotSame(first, second)
	line, _ := second.ReadString('\n')
	s.Equal("b\n", line)

	r, w, err := os.Pipe()
	s.Require().NoError(err)
	defer r.Close()
	defer w.Close()
	s.Same(Reader(r), Reader(r))
}

// A replaced os.Stdin is read from the start, with nothing the reader on the
// old one read ahead carried over.
func (s *Suite) TestReaderFollowsAReplacedStdin() {
	defer SetGuard(nil)()
	s.stdinWith("old\nstale\n")
	got, err := Text("")
	s.Require().NoError(err)
	s.Equal("old", got)
	s.stdinWith("new\n")
	got, err = Text("")
	s.Require().NoError(err)
	s.Equal("new", got)
}
