package local

import (
	"github.com/astronomer/astro-cli/pkg/input"
)

// confirmUnless asks before something destructive, unless the caller already
// said yes. It is the whole guard: safe by default, one flag to skip, and a
// run that cannot be asked and did not pass the flag fails rather than
// proceeding.
func (c *cli) confirmUnless(yes bool, question string) error {
	if yes {
		return nil
	}
	return c.confirm(question)
}

// confirm asks a yes/no question, defaulting to no, through the shared
// confirmation every command uses (input.Confirmation): a run that may not ask
// is refused before anything is written or read, a closed or empty stdin is an
// input_required error naming --yes rather than a silent "no", any other
// answer but a yes is input.ErrAborted, and Ctrl-C ends the question at once
// with input.ErrInterrupted. This only supplies astro local's streams and its
// "[y/N]" wording.
func (c *cli) confirm(question string) error {
	return (&input.Confirmation{
		Question: question,
		Choices:  " [y/N] ",
		In:       c.d.Stdin,
		Out:      c.d.Stderr,
		Options:  []input.Option{input.AnsweredBy("--yes")},
	}).Ask()
}
