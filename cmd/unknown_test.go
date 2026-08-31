package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// newUnknownTestTree builds a command tree shaped like the real one: a root
// that does not run, a parent that does not run, and commands that run.
func newUnknownTestTree() *cobra.Command {
	root := &cobra.Command{Use: "astro"}
	root.PersistentFlags().String("verbosity", "warn", "log level")

	dev := &cobra.Command{Use: "dev"}
	dev.PersistentFlags().Bool("no-cache", false, "do not use a cache")
	dev.AddCommand(
		&cobra.Command{Use: "start", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "restart", Run: func(*cobra.Command, []string) {}},
	)

	root.AddCommand(
		dev,
		&cobra.Command{Use: "deploy", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "telemetry", Run: func(*cobra.Command, []string) {}},
	)
	return root
}

func TestFindUnknownCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantParent string
		wantWord   string
	}{
		{"unknown at root", []string{"fly"}, "astro", "fly"},
		{"unknown under a parent", []string{"dev", "restrt"}, "astro dev", "restrt"},
		{"unknown after a flag with a value", []string{"--verbosity", "debug", "fly"}, "astro", "fly"},
		{"unknown after a boolean flag", []string{"dev", "--no-cache", "restrt"}, "astro dev", "restrt"},
		{"unknown with a flag after it", []string{"fly", "--verbosity", "debug"}, "astro", "fly"},
		{"known command", []string{"dev", "start"}, "", ""},
		{"known parent alone", []string{"dev"}, "", ""},
		{"argument to a command that runs", []string{"deploy", "my-deployment"}, "", ""},
		{"no arguments", []string{}, "", ""},
		{"flags only", []string{"--verbosity", "debug"}, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unknown := findUnknownCommand(newUnknownTestTree(), tt.args)

			if tt.wantWord == "" {
				assert.Nil(t, unknown)
				return
			}
			assert.Equal(t, tt.wantWord, unknown.word)
			assert.Equal(t, tt.wantParent, unknown.parent.CommandPath())
		})
	}
}

func TestUnknownCommandSuggestion(t *testing.T) {
	root := newUnknownTestTree()

	assert.Equal(t, "deploy", findUnknownCommand(root, []string{"deploi"}).suggestion())
	assert.Equal(t, "telemetry", findUnknownCommand(root, []string{"telem"}).suggestion())
	assert.Equal(t, "restart", findUnknownCommand(root, []string{"dev", "restrt"}).suggestion())
	assert.Empty(t, findUnknownCommand(root, []string{"fly"}).suggestion())

	root.DisableSuggestions = true
	assert.Empty(t, findUnknownCommand(root, []string{"deploi"}).suggestion())
}

// TestHandleUnknownCommandMatchesCobra checks our wording against the wording
// cobra prints for the same mistake at the root.
func TestHandleUnknownCommandMatchesCobra(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	tests := []struct {
		name string
		args []string
	}{
		{"no suggestion", []string{"fly"}},
		{"with a suggestion", []string{"deploi"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cobraRoot := newUnknownTestTree()
			fromCobra := new(bytes.Buffer)
			cobraRoot.SetOut(fromCobra)
			cobraRoot.SetErr(fromCobra)
			cobraRoot.SetArgs(tt.args)
			require.Error(t, cobraRoot.Execute())

			ours := new(bytes.Buffer)
			assert.True(t, HandleUnknownCommand(newUnknownTestTree(), tt.args, ours))

			assert.Equal(t, fromCobra.String(), ours.String())
		})
	}
}

func TestHandleUnknownCommandUnderAParent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	out := new(bytes.Buffer)

	assert.True(t, HandleUnknownCommand(newUnknownTestTree(), []string{"dev", "restrt"}, out))
	assert.Equal(t, "Error: unknown command \"restrt\" for \"astro dev\"\n\nDid you mean this?\n\trestart\n\nRun 'astro dev --help' for usage.\n", out.String())
}

func TestHandleUnknownCommandLeavesKnownCommandsAlone(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	out := new(bytes.Buffer)

	assert.False(t, HandleUnknownCommand(newUnknownTestTree(), []string{"dev", "start"}, out))
	assert.Empty(t, out.String())
}

// TestHandleUnknownCommandLeavesCobrasOwnCommandsAlone guards the commands
// cobra adds inside Execute, after the point where we resolve the tree.
func TestHandleUnknownCommandLeavesCobrasOwnCommandsAlone(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	tests := [][]string{
		{"help"},
		{"help", "dev"},
		{"help", "fly"},
		{"completion"},
		{"completion", "zsh"},
		{"__complete", "dev", "res"},
		{"__completeNoDesc", "dev", "res"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out := new(bytes.Buffer)

			assert.False(t, HandleUnknownCommand(newUnknownTestTree(), args, out))
			assert.Empty(t, out.String())
		})
	}
}

// TestHandleUnknownCommandOnTheRealTree runs against the command tree the CLI
// actually builds, on both platforms, since each builds a different one.
func TestHandleUnknownCommandOnTheRealTree(t *testing.T) {
	platforms := []struct {
		name     string
		platform string
	}{
		{"cloud", testUtil.LocalPlatform},
		{"software", testUtil.SoftwarePlatform},
	}

	for _, p := range platforms {
		t.Run(p.name, func(t *testing.T) {
			testUtil.InitTestConfig(p.platform)
			root := NewRootCmd()
			out := new(bytes.Buffer)

			assert.True(t, HandleUnknownCommand(root, []string{"fly"}, out))
			assert.Equal(t, "Error: unknown command \"fly\" for \"astro\"\nRun 'astro --help' for usage.\n", out.String())

			for _, args := range [][]string{{"version"}, {"help"}, {"completion", "zsh"}, {"__complete", "dev", "res"}, {}} {
				out := new(bytes.Buffer)
				assert.False(t, HandleUnknownCommand(NewRootCmd(), args, out), args)
				assert.Empty(t, out.String())
			}
		})
	}
}
