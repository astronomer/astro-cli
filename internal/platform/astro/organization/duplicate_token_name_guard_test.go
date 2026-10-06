package organization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
)

// A name several tokens share needs a choice. A run that may not ask (any
// command under -o json) is refused before the picker is, naming the token's
// ID as the answer: the name, which is what was given, answers nothing.
func TestDuplicateTokenNameRefusedBeforeThePicker(t *testing.T) {
	t.Cleanup(input.SetGuard(func() string { return "with --output json it cannot" }))
	tokens := []astrov1.ApiToken{{Id: "a", Name: "dup"}, {Id: "b", Name: "dup"}}
	pick := func(string, []apitoken.Token) (int, error) {
		t.Fatal("the picker was asked, though the run may not ask")
		return 0, nil
	}

	_, err := getOrganizationToken("", "dup", tokens, pick)

	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "the token's ID instead of its name")
}
