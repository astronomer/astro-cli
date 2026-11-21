package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/version"
)

type lineRun struct {
	stdout, stderr string
	code           int
	reachedLogin   bool
}

// runLine runs args through a real Astro root, with the login and version check
// replaced by a refusal that records it was reached.
func runLine(t *testing.T, args ...string) lineRun {
	t.Helper()
	var run lineRun
	orig := platformPreRun
	platformPreRun = func(astrov1.APIClient) func(*cobra.Command, []string) error {
		return func(*cobra.Command, []string) error {
			run.reachedLogin = true
			return errors.New("login reached")
		}
	}
	t.Cleanup(func() { platformPreRun = orig })

	root := buildTree(t, astroTree).root
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := execute(t.Context(), root, args, &out)
	run.stdout, run.stderr, run.code = out.String(), errOut.String(), cliout.ExitCode(t.Context(), err)
	return run
}

func versionLine(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, version.PrintVersion(&b))
	return b.String()
}

// --version prints what `astro version` prints, at any depth, before the login.
// There is no -v: it is --verbose's letter on several commands.
func TestVersionFlagPrintsTheVersionAnywhere(t *testing.T) {
	want := versionLine(t)
	for _, args := range [][]string{
		{"--version"},
		{"version", "--version"},
		{"deployment", "--version"},
		{"deployment", "list", "--version"},
		{"deployment", "list", "--version=true"},
		{"local", "start", "--version"},
		{"--version", "extra"},
	} {
		run := runLine(t, args...)
		assert.Equal(t, want, run.stdout, args)
		assert.Zero(t, run.code, args)
		assert.False(t, run.reachedLogin, args)
	}
}

// What is not a request for the version: help outranks it, --version=false
// asks for nothing, a word after -- or inside a program's argv is not ours.
func TestVersionFlagIsLeftAloneWhereItIsNotOurs(t *testing.T) {
	want := versionLine(t)
	for _, args := range [][]string{
		{"--version", "--help"},
		{"deployment", "list", "-h", "--version"},
		{"--help", "--version=true"},
	} {
		run := runLine(t, args...)
		assert.NotEqual(t, want, run.stdout, args)
		assert.Contains(t, run.stdout, "Usage:", args)
		assert.Zero(t, run.code, args)
	}
	for _, args := range [][]string{
		{"deployment", "list", "--", "--version"},
		{"local", "run", "python", "--version"},
		{"deployment", "list", "--version=false"},
	} {
		assert.NotEqual(t, want, runLine(t, args...).stdout, args)
	}
}

func TestShortVersionFlagDoesNotExist(t *testing.T) {
	run := runLine(t, "-v")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, "unknown shorthand flag: 'v'")
}

// Help wins over a flag error: a line that asks for help gets it, whatever
// else is wrong with its flags.
func TestHelpWinsOverFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "list", "--bogus", "--help"},
		{"deployment", "list", "-h", "--bogus"},
		{"deployment", "list", "--bogus", "-h"},
		{"--bogus", "-h"},
		{"deployment", "list", "-o", "nonsense", "--help"},
		{"deployment", "list", "--help", "-o", "nonsense"},
	} {
		run := runLine(t, args...)
		assert.Zero(t, run.code, "%v: %s", args, run.stderr)
		assert.Contains(t, run.stdout, "Usage:", args)
		assert.Empty(t, run.stderr, args)
	}
	// After --, -h is not a request for help.
	run := runLine(t, "deployment", "list", "--bogus", "--", "-h")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, "unknown flag: --bogus")
}

func TestHelpForAWordThatIsNoCommandIsAUsageError(t *testing.T) {
	run := runLine(t, "help", "nonexist")
	assert.Equal(t, 2, run.code)
	assert.Empty(t, run.stdout)
	assert.Contains(t, run.stderr, `unknown command "nonexist" for "astro"`)
	assert.Contains(t, run.stderr, "Run astro --help for usage.")
	assert.NotContains(t, run.stderr, "Unknown help topic")

	run = runLine(t, "help", "deployment", "nonexist")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, `unknown command "nonexist" for "astro deployment"`)
	assert.Contains(t, run.stderr, "Run astro deployment --help for usage.")

	run = runLine(t, "help", "deployment", "lst")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, "Did you mean this?\n\tlist")

	for _, args := range [][]string{{"help"}, {"help", "deployment"}, {"help", "deployment", "ls"}} {
		run = runLine(t, args...)
		assert.Zero(t, run.code, args)
		assert.Contains(t, run.stdout, "Usage:", args)
	}
}

// A leaf takes no word it does not document and a group no word that is not
// one of its subcommands, and the refusal comes before the login.
func TestStrayWordsAreUsageErrorsBeforeTheLogin(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"version", "extra"}, `unknown command "extra" for "astro version"`},
		{[]string{"telemetry", "bogus"}, `unknown command "bogus" for "astro telemetry"`},
		{[]string{"telemetry", "enable", "extra"}, `unknown command "extra" for "astro telemetry enable"`},
		{[]string{"completion", "bogus"}, `unknown command "bogus" for "astro completion"`},
		{[]string{"deployment", "list", "extra"}, `unknown command "extra" for "astro deployment list"`},
		{[]string{"env", "bogus"}, `unknown command "bogus" for "astro env"`},
		{[]string{"env", "variable", "bogus"}, `unknown command "bogus" for "astro env variable"`},
		{[]string{"context", "list", "extra"}, `unknown command "extra" for "astro context list"`},
		{[]string{"deployment", "delete", "a", "b"}, "accepts at most 1 arg(s), received 2"},
	} {
		run := runLine(t, tc.args...)
		assert.Equal(t, 2, run.code, tc.args)
		assert.Contains(t, run.stderr, tc.want, tc.args)
		assert.False(t, run.reachedLogin, tc.args)
	}
}

// A documented positional still works: the refusal is only of what is not one.
func TestDocumentedPositionalsStillReachTheirCommand(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "delete", "clx0000000000000000000001"},
		{"deployment", "team", "remove", "team-eng", "--deployment", "clx0000000000000000000001"},
		{"deployment", "user", "remove", "ada@example.com", "--deployment", "clx0000000000000000000001"},
		{"deployment", "variable", "create", "A=1", "B=2"},
	} {
		run := runLine(t, args...)
		assert.True(t, run.reachedLogin, "%v: %s", args, run.stderr)
	}
}

// A mistyped command suggests the one command it is clearly close to, and none
// when two are as near.
func TestUnknownCommandSuggestsAtMostOne(t *testing.T) {
	run := runLine(t, "depl")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, "Did you mean this?\n\tdeploy\n")
	assert.NotContains(t, run.stderr, "\tdeployment")

	assert.Equal(t, []string{"deploy"}, cliout.SubcommandSuggestions(buildTree(t, astroTree).root, "deploi"))

	tied := &cobra.Command{Use: "x"}
	tied.AddCommand(&cobra.Command{Use: "abcx", Run: func(*cobra.Command, []string) {}}, &cobra.Command{Use: "abcy", Run: func(*cobra.Command, []string) {}})
	assert.Empty(t, cliout.SubcommandSuggestions(tied, "abc"))
}

// A level set in ASTRO_VERBOSITY that is no level fails the run as --verbosity
// does, naming the variable, before the login.
func TestBadVerbosityEnvironmentIsAUsageError(t *testing.T) {
	keepLogger(t)
	t.Setenv(verbosityEnv, "bogus")
	run := runLine(t, "version")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stderr, `ASTRO_VERBOSITY: "bogus" is not a log level`)
	assert.False(t, run.reachedLogin)

	run = runLine(t, "--verbosity", "bogus", "version")
	assert.Equal(t, 2, run.code)
}
