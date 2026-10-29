package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// example-visible reports what an example uses that help does not show: a
// hidden flag, by its name whichever spelling the line typed, and a hidden
// command. A line example-runs refuses is left to that rule.
func TestCheckExampleVisible(t *testing.T) {
	tree := func(example string) *cobra.Command {
		root := &cobra.Command{Use: "astro"}
		group := &cobra.Command{Use: "deployment"}
		create := &cobra.Command{Use: "create", Example: example, Run: func(*cobra.Command, []string) {}}
		create.Flags().StringP("name", "n", "", "Name")
		create.Flags().StringP("label", "l", "", "Old spelling")
		create.Flags().Lookup("label").Hidden = true
		logs := &cobra.Command{Use: "logs", Run: func(*cobra.Command, []string) {}}
		logs.AddCommand(&cobra.Command{Use: "scheduler", Hidden: true, Run: func(*cobra.Command, []string) {}})
		group.AddCommand(create, logs)
		root.AddCommand(group)
		return create
	}
	for example, want := range map[string]string{
		"  astro deployment create --name prod":                             "",
		"  astro deployment create -n prod":                                 "",
		"  # astro deployment create --label prod":                          "",
		"  astro deployment create --label prod":                            `"astro deployment create --label prod" uses the hidden flag --label`,
		"  astro deployment create -l prod":                                 `"astro deployment create -l prod" uses the hidden flag --label`,
		"  astro deployment logs scheduler":                                 `"astro deployment logs scheduler" uses the hidden command "astro deployment logs scheduler"`,
		"  astro deployment create --nonesuch x":                            "",
		"  astro deployment create \\\n    --label prod":                    `"astro deployment create      --label prod" uses the hidden flag --label`,
		"  astro deployment create -n a && astro deployment logs scheduler": `"astro deployment create -n a && astro deployment logs scheduler" uses the hidden command "astro deployment logs scheduler"`,
	} {
		assert.Equal(t, want, checkExampleVisible(tree(example)), example)
	}
}
