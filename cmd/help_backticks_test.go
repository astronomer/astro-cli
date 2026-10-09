package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Every help page, hidden commands' included, is plain text: a command, flag
// or path in a Short, a Long, an example or a flag's description is written
// without backticks, which a terminal prints as they are.
// TestMessagesWriteCommandsAsPlainText in internal/archlint holds the source
// to the same rule; this reads what the pages print, built from strings
// assembled at run time too.
func TestHelpHasNoBackticks(t *testing.T) {
	pinHelpWidth(t)
	for _, tree := range rootsUnderTest(t) {
		walkTree(tree.root, func(c *cobra.Command) {
			var page strings.Builder
			c.SetOut(&page)
			c.HelpFunc()(c, nil)
			for _, line := range strings.Split(page.String(), "\n") {
				if strings.Contains(line, "`") {
					t.Errorf("%s: %q: a help line holds a backtick; write the command as plain text: %q",
						tree.name, c.CommandPath(), line)
				}
			}
		})
	}
}
