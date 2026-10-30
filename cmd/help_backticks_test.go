package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Every help page, hidden commands' included, is plain text, and so is every
// flag's usage as written: a command, flag or path in a Short, a Long, an
// example or a flag's description is written without backticks, which a
// terminal prints as they are.
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
			// pflag takes a backticked word in a flag's usage as the flag's
			// value name and drops the backticks from the page, so the page
			// alone cannot show one: read the usage as written.
			c.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if strings.Contains(f.Usage, "`") {
					t.Errorf("%s: %q: --%s's usage holds a backtick; write the command as plain text: %q",
						tree.name, c.CommandPath(), f.Name, f.Usage)
				}
			})
		})
	}
}

func walkTree(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkTree(sub, fn)
	}
}
