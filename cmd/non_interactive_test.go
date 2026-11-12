package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
)

// --non-interactive is registered once, persistent, at the root, so every
// command at every depth in every tree accepts it, the removed-command stubs
// and the hidden ones included.
func TestEveryCommandAcceptsNonInteractive(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		t.Run(tree.name, func(t *testing.T) {
			require.NotNil(t, tree.root.PersistentFlags().Lookup(input.NonInteractiveFlag), "the root declares it")
			walkTree(tree.root, func(c *cobra.Command) {
				if c != tree.root && c.Parent() == nil {
					return // a command the version gate cut out of the tree: unreachable
				}
				assert.NotNil(t, c.Flag(input.NonInteractiveFlag), "%s does not take --%s", c.CommandPath(), input.NonInteractiveFlag)
			})
		})
	}
}

// A removed command's stub parses no flags, so the flag reaches it as an
// argument and changes nothing: it still says what replaced the command. The
// completion command is the cobra one, untouched.
func TestNonInteractiveDoesNotChangeStubsOrCompletion(t *testing.T) {
	trees := rootsUnderTest(t)
	root := treeNamed(t, trees, trees[0].name)

	_, _, err := executeRoot(root, "dev", "start", "--non-interactive")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was removed in Astro CLI v2")

	stdout, _, err := executeRoot(root, "__complete", "version", "")
	require.NoError(t, err)
	assert.NotContains(t, stdout, input.NonInteractiveFlag+"\n", "a completion of arguments is not a flag list")
}
