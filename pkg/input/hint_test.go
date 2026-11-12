package input

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A command that knows what answers a shared picker says so once, and every
// refusal about that question names it, whatever the reason; an asker that
// names its own answer keeps it.
func TestHintedAnswersAreNamedInRefusals(t *testing.T) {
	t.Cleanup(ClearHints)
	HintAnswer("a user", "the email as an argument")

	e := ApplyHint(&RequiredError{About: "a user", Reason: EndedReason})
	assert.Equal(t, "the email as an argument", e.Flag)
	assert.Contains(t, e.Error(), "pass the email as an argument")

	t.Cleanup(SetGuard(func() string { return "with CI set it cannot" }))
	err := MayAsk("\n> ", About("a user"))
	assert.ErrorContains(t, err, "pass the email as an argument")

	own := ApplyHint(&RequiredError{About: "a user", Flag: "--user-id"})
	assert.Equal(t, "--user-id", own.Flag, "the asker's own answer wins")
	other := ApplyHint(&RequiredError{About: "a team"})
	assert.Empty(t, other.Flag, "a hint is for its own question only")

	ClearHints()
	assert.Empty(t, ApplyHint(&RequiredError{About: "a user"}).Flag)
}
