package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// --no-color is registered once, persistent, at the root, so every command at
// every depth in every tree accepts it, `astro api` included: one flag with
// one meaning.
func TestEveryCommandAcceptsNoColor(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		t.Run(tree.name, func(t *testing.T) {
			require.NotNil(t, tree.root.PersistentFlags().Lookup(ansi.NoColorFlag), "the root declares it")
			walkTree(tree.root, func(c *cobra.Command) {
				if c != tree.root && c.Parent() == nil {
					return
				}
				assert.NotNil(t, c.Flag(ansi.NoColorFlag), "%s does not take --%s", c.CommandPath(), ansi.NoColorFlag)
			})
		})
	}
}
