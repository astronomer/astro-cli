package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI's prose writes a command the way its docs do, in backticks, and the
// output layer renders them (ansi.Backticks): bold on a terminal, plain text
// anywhere else. These tests read that output where a pipe or a script would.

// plainOutput turns off every switch that colors a stream whatever it is, so
// a buffer reads as the pipe it stands in for.
func plainOutput(t *testing.T) {
	t.Helper()
	for _, v := range []string{"CLICOLOR_FORCE", "CLICOLOR", "NO_COLOR"} {
		t.Setenv(v, "")
		require.NoError(t, os.Unsetenv(v))
	}
}

// Every help page, hidden commands' included, prints no backtick and no color
// to a pipe, in every tree.
func TestHelpPrintsNoBackticksToAPipe(t *testing.T) {
	plainOutput(t)
	pinHelpWidth(t)
	for _, tree := range rootsUnderTest(t) {
		walkTree(tree.root, func(c *cobra.Command) {
			var page strings.Builder
			c.SetOut(&page)
			c.HelpFunc()(c, nil)
			if strings.ContainsAny(page.String(), "`\x1b") {
				for _, line := range strings.Split(page.String(), "\n") {
					if strings.ContainsAny(line, "`\x1b") {
						t.Errorf("%s: %q: help for a pipe holds a backtick or an escape: %q", tree.name, c.CommandPath(), line)
					}
				}
			}
		})
	}
}

// On a terminal a page shows the same span in bold.
func TestHelpShowsBackticksInBoldOnATerminal(t *testing.T) {
	plainOutput(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	pinHelpWidth(t)
	root := treeNamed(t, rootsUnderTest(t), "astro")
	dev, _, err := root.Find([]string{"dev"})
	require.NoError(t, err)
	var page strings.Builder
	dev.SetOut(&page)
	dev.HelpFunc()(dev, nil)
	assert.Contains(t, page.String(), "\x1b[1mastro local\x1b[0m")
	assert.NotContains(t, page.String(), "`")
}

// A failure's message reaches stderr as plain text, and the error object
// under --output json carries plain text too: removed commands' stubs, and a
// usage error. `astro dev` publishes a payload of its own under json, to the
// command's stdout rather than Execute's, so only its text is read here.
func TestErrorsPrintNoBackticks(t *testing.T) {
	plainOutput(t)
	for _, c := range []struct {
		name string
		args []string
		want string
		json bool
	}{
		{"a removed command", []string{"dev", "start"}, "astro dev start was removed in Astro CLI v2. Use astro local start instead.", false},
		{"another removed command", []string{"run", "my_dag"}, "astro run was removed in Astro CLI v2. Use astro local run airflow dags test my_dag instead.", true},
		{"a usage error", []string{"local", "init"}, `unknown command "init" for "astro local". Use astro init`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := treeNamed(t, treesToExecute(t), "astro")
			stdout, stderr, err := executeRoot(root, c.args...)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "Error: "+c.want)
			assert.NotContains(t, stderr, "`")
			assert.NotContains(t, stderr, "\x1b")
			if !c.json {
				return
			}

			root = treeNamed(t, treesToExecute(t), "astro")
			stdout, stderr, err = executeRoot(root, append(c.args, "-o", "json")...)
			require.Error(t, err)
			assert.Empty(t, stderr)
			var obj struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &obj), stdout)
			assert.NotEmpty(t, obj.Error)
			assert.NotContains(t, obj.Error, "`", "the error object's message is plain text")
			assert.True(t, strings.HasPrefix(c.want, obj.Error) || strings.HasPrefix(obj.Error, strings.TrimSuffix(c.want, ".")), "%q", obj.Error)
		})
	}
}
