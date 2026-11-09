package input

import "os"

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
		got := ConfirmOrAbort("Delete it?", AnsweredBy("--yes"))
		s.True(IsRequired(got), "%v", got)
		s.NotErrorIs(got, ErrAborted)
		s.Contains(got.Error(), "pass --yes")
	})
}
