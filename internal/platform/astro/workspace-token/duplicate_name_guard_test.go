package workspacetoken

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
)

// A name several tokens share needs a choice. A run that may not ask (any
// command under -o json) is refused before the heading is printed, so stdout
// carries nothing but the command's own object.
func TestDuplicateTokenNameRefusedBeforeTheHeading(t *testing.T) {
	t.Cleanup(input.SetGuard(func() string { return "with --output json it cannot" }))
	tokens := []astrov1.ApiToken{{Id: "a", Name: "dup"}, {Id: "b", Name: "dup"}}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	_, err = getWorkspaceToken("", "dup", "ws", "", tokens)
	os.Stdout = stdout
	w.Close()
	printed, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "the token's ID instead of its name")
	assert.Empty(t, string(printed), "nothing reaches stdout")
}
