package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// TestFindUnknownCommandLeavesCobrasOwnCommandsAlone guards the commands cobra
// adds inside Execute. Execute looks for an unknown command after the run, so
// they are in the tree by then; this adds them the way cobra does.
func TestFindUnknownCommandLeavesCobrasOwnCommandsAlone(t *testing.T) {
	tests := [][]string{
		{"help"},
		{"help", "dev"},
		{"help", "fly"},
		{"completion"},
		{"completion", "zsh"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := newUnknownTestTree()
			root.InitDefaultHelpCmd()
			root.InitDefaultCompletionCmd(args...)

			assert.Nil(t, findUnknownCommand(root, args))
		})
	}
}

func TestIsShellCompletion(t *testing.T) {
	assert.True(t, isShellCompletion([]string{"__complete", "dev", "res"}))
	assert.True(t, isShellCompletion([]string{"__completeNoDesc", "dev", "res"}))
	assert.False(t, isShellCompletion([]string{"dev", "res"}))
	assert.False(t, isShellCompletion(nil))
}

// TestFindUnknownCommandOnTheRealTree runs against the command tree the CLI
// actually builds, on both platforms, since each builds a different one, with
// the commands cobra adds during a run, which is when Execute looks.
func TestFindUnknownCommandOnTheRealTree(t *testing.T) {
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

			for args, want := range map[string]string{"fly": "fly", "deploymnt": "deploymnt"} {
				root := NewRootCmd()
				root.InitDefaultHelpCmd()
				root.InitDefaultCompletionCmd(args)

				unknown := findUnknownCommand(root, []string{args})
				require.NotNil(t, unknown, args)
				assert.Equal(t, want, unknown.word)
				assert.Equal(t, "astro", unknown.parent.CommandPath())
			}

			for _, args := range [][]string{{"version"}, {"help"}, {"completion", "zsh"}, {}} {
				root := NewRootCmd()
				root.InitDefaultHelpCmd()
				root.InitDefaultCompletionCmd(args...)

				assert.Nil(t, findUnknownCommand(root, args), args)
			}
		})
	}
}

// parseError returns the error pflag gives for args, which is the error cobra
// hands to the flag error function.
func parseError(t *testing.T, args ...string) error {
	t.Helper()

	flags := pflag.NewFlagSet("astro", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.String("verbosity", "warn", "log level")
	flags.BoolP("all", "a", false, "every one of them")

	return flags.Parse(args)
}

func TestUnknownFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"a long flag we do not have", []string{"--wait-for-deploy"}, "--wait-for-deploy"},
		{"a long flag with a value", []string{"--api-token=sekret-123"}, "--api-token"},
		{"a shorthand we do not have", []string{"-Z"}, "-Z"},
		{"a shorthand inside a group", []string{"-aZ"}, "-Z"},
		{"a flag we do have", []string{"--verbosity", "debug"}, ""},
		{"a flag missing its value", []string{"--verbosity"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, spelling := unknownFlag(parseError(t, tt.args...))
			assert.Equal(t, tt.want, spelling)
		})
	}

	for _, err := range []error{nil, errors.New("something else")} {
		name, isShorthand, spelling := unknownFlag(err)
		assert.Empty(t, name)
		assert.False(t, isShorthand)
		assert.Empty(t, spelling)
	}
	name, isShorthand, _ := unknownFlag(parseError(t, "-aZ"))
	assert.Equal(t, "Z", name)
	assert.True(t, isShorthand)
	name, isShorthand, _ = unknownFlag(parseError(t, "--api-token=sekret-123"))
	assert.Equal(t, "api-token", name)
	assert.False(t, isShorthand)
}

// TestTrackUnknownFlagPassesTheErrorThrough checks that the hook only listens.
// Cobra reports the error, and it has to arrive unchanged.
func TestTrackUnknownFlagPassesTheErrorThrough(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	err := parseError(t, "--wait-for-deploy")
	cmd := &cobra.Command{Use: "deploy"}

	assert.Equal(t, err, trackUnknownFlag(cmd, err))
}

// TestUnknownFlagOnTheRealTree checks that the root's error function reaches a
// command several levels down, and that the CLI still reports what it always
// reported.
func TestUnknownFlagOnTheRealTree(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	root := NewRootCmd()
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"deployment", "list", "--wait-for-deploy"})

	err := root.Execute()

	require.Error(t, err)
	assert.Equal(t, "unknown flag: --wait-for-deploy", err.Error())
	assert.Contains(t, out.String(), "Error: unknown flag: --wait-for-deploy")
}
