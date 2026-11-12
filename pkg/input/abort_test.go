package input

import (
	"io"
	"os"
)

// stdinHolding makes os.Stdin a pipe holding answer, then closed, until the
// subtest ends.
func (s *Suite) stdinHolding(answer string) {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.WriteString(answer)
	s.Require().NoError(err)
	s.Require().NoError(w.Close())
	saved := os.Stdin
	os.Stdin = r
	s.T().Cleanup(func() { os.Stdin = saved })
}

// A yes is y or yes in any case; anything else answered is ErrAborted; and
// input that ends before an answer is nobody answering, a *RequiredError
// naming the flag, never a no.
func (s *Suite) TestConfirmOrAbort() {
	for answer, want := range map[string]error{
		"y\n": nil, "Y\n": nil, "yes\n": nil, "YES\r\n": nil, "y": nil,
		"n\n": ErrAborted, "no\n": ErrAborted, "\n": ErrAborted, "maybe\n": ErrAborted, "n": ErrAborted,
	} {
		s.Run(answer, func() {
			s.stdinHolding(answer)
			got := ConfirmOrAbort("Delete it?", AnsweredBy("--yes"))
			if want == nil {
				s.NoError(got)
			} else {
				s.ErrorIs(got, want)
			}
		})
	}

	s.Run("ended input", func() {
		s.stdinHolding("")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		stderr := os.Stderr
		os.Stderr = w
		got := ConfirmOrAbort("Delete it?", AnsweredBy("--yes"))
		os.Stderr = stderr
		s.Require().NoError(w.Close())
		asked, err := io.ReadAll(r)
		s.Require().NoError(err)
		s.Equal("Delete it? (y/n) \n", string(asked), "the prompt's line is ended before the error")
		s.True(IsRequired(got), "%v", got)
		s.NotErrorIs(got, ErrAborted)
		s.Contains(got.Error(), "pass --yes")
	})
}

// A Text question whose input ends with nothing is nobody answering: a
// *RequiredError naming the flag, not an empty answer. Text typed before the
// end is still the answer.
func (s *Suite) TestTextEnded() {
	s.stdinHolding("")
	_, err := Text("Name: ", AnsweredBy("--name"))
	s.True(IsRequired(err), "%v", err)
	s.Contains(err.Error(), "pass --name")

	s.stdinHolding("prod")
	got, err := Text("Name: ", AnsweredBy("--name"))
	s.NoError(err)
	s.Equal("prod", got)

	s.stdinHolding("\n")
	got, err = Text("Name: ", AnsweredBy("--name"))
	s.NoError(err)
	s.Empty(got, "an empty line is an answer")
}

// A password asked of a stdin that ends names the flag that answers it.
func (s *Suite) TestPasswordEndedNamesTheFlag() {
	s.stdinHolding("")
	_, err := Password("Password: ", AnsweredBy("--password"))
	s.True(IsRequired(err), "%v", err)
	s.Contains(err.Error(), "pass --password")
}
