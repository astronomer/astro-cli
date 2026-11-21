package cliout

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leafWith(use string, args ...Arg) *cobra.Command {
	return Args(&cobra.Command{Use: use, RunE: func(*cobra.Command, []string) error { return nil }}, args...)
}

func TestArgsWritesTheUseLineFromTheDeclaration(t *testing.T) {
	for want, cmd := range map[string]*cobra.Command{
		"version":                    leafWith("version"),
		"get <NAME>":                 leafWith("get", Required("NAME")),
		"get [NAME]":                 leafWith("get", Optional("NAME")),
		"get <A> [B]":                leafWith("get", Required("A"), Optional("B")),
		"rm <NAME>...":               leafWith("rm", Arg{Name: "NAME", Repeated: true}),
		"rm [NAME...]":               leafWith("rm", Arg{Name: "NAME", Optional: true, Repeated: true}),
		"add [NAME] [-- COMMAND...]": leafWith("add", Optional("NAME"), Arg{Name: "COMMAND", Optional: true, Repeated: true, AfterDash: true}),
		"run -- <COMMAND>...":        leafWith("run", Arg{Name: "COMMAND", Repeated: true, AfterDash: true}),
	} {
		assert.Equal(t, want, cmd.Use)
	}
}

func TestArgsDeclarationReadsBack(t *testing.T) {
	declared := []Arg{Required("A"), Optional("B"), {Name: "C", Optional: true, Repeated: true}, {Name: "KEY=VALUE", Repeated: true, AfterDash: true}}
	cmd := leafWith("x", declared...)
	assert.Equal(t, declared, DeclaredArgs(cmd))
	assert.Empty(t, DeclaredArgs(&cobra.Command{Use: "y"}))
}

func TestArgsRefusesWhatNoLineCouldSatisfy(t *testing.T) {
	for name, build := range map[string]func(){
		"a Use that already has positionals": func() { leafWith("get <NAME>") },
		"required after optional":            func() { leafWith("x", Optional("A"), Required("B")) },
		"anything after a repeated one":      func() { leafWith("x", Arg{Name: "A", Repeated: true}, Optional("B")) },
		"a dash that is not last":            func() { leafWith("x", Arg{Name: "A", AfterDash: true}, Optional("B")) },
		"a name that is not a placeholder":   func() { leafWith("x", Required("<A>")) },
		"an empty name":                      func() { leafWith("x", Required("")) },
		"a colon, which the annotation uses": func() { leafWith("x", Required("A:B")) },
	} {
		assert.Panics(t, build, name)
	}
}

// The wording of a refusal, for a leaf that takes none, one optional, one
// required, a range, and a repeated argument.
func TestArgsRefusalWording(t *testing.T) {
	for name, tc := range map[string]struct {
		cmd  *cobra.Command
		args []string
		want string // "" accepts
	}{
		"none, none given":         {leafWith("version"), nil, ""},
		"none, one given":          {leafWith("version"), []string{"x"}, `unexpected argument "x": root version takes no arguments`},
		"optional, none given":     {leafWith("get", Optional("ID")), nil, ""},
		"optional, one given":      {leafWith("get", Optional("ID")), []string{"a"}, ""},
		"optional, two given":      {leafWith("get", Optional("ID")), []string{"a", "b"}, `unexpected argument "b": root get takes at most 1 argument`},
		"required, none given":     {leafWith("get", Required("ID")), nil, "root get requires ID"},
		"required, one given":      {leafWith("get", Required("ID")), []string{"a"}, ""},
		"required, two given":      {leafWith("get", Required("ID")), []string{"a", "b"}, `unexpected argument "b": root get takes exactly 1 argument`},
		"two required, one given":  {leafWith("set", Required("NAME"), Required("VALUE")), []string{"a"}, "root set requires VALUE"},
		"two required, none given": {leafWith("set", Required("NAME"), Required("VALUE")), nil, "root set requires NAME and VALUE"},
		"two required, three":      {leafWith("set", Required("NAME"), Required("VALUE")), []string{"a", "b", "c"}, `unexpected argument "c": root set takes exactly 2 arguments`},
		"three required, none": {
			leafWith("x", Required("A"), Required("B"), Required("C")), nil, "root x requires A, B and C",
		},
		"range, none given":  {leafWith("x", Required("A"), Optional("B")), nil, "root x requires A"},
		"range, three given": {leafWith("x", Required("A"), Optional("B")), []string{"a", "b", "c"}, `unexpected argument "c": root x takes 1 to 2 arguments`},
		"range, two given":   {leafWith("x", Required("A"), Optional("B")), []string{"a", "b"}, ""},
		"repeated, none":     {leafWith("rm", Arg{Name: "NAME", Repeated: true}), nil, "root rm requires NAME"},
		"repeated, many":     {leafWith("rm", Arg{Name: "NAME", Repeated: true}), []string{"a", "b", "c", "d"}, ""},
		"optional repeated":  {leafWith("rm", Arg{Name: "NAME", Optional: true, Repeated: true}), nil, ""},
		"required then repeated, one given": {
			leafWith("clear", Required("DAG"), Arg{Name: "TASK", Repeated: true}), []string{"a"}, "root clear requires TASK",
		},
	} {
		root := &cobra.Command{Use: "root"}
		root.AddCommand(tc.cmd)
		r := execTree(context.Background(), root, append([]string{tc.cmd.Name()}, tc.args...)...)
		if tc.want == "" {
			assert.NoError(t, r.err, name)
			continue
		}
		require.Error(t, r.err, name)
		assert.EqualError(t, r.err, tc.want, name)
		assert.True(t, IsUsage(r.err), name)
		assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err), name)
		assert.Equal(t, "Error: "+tc.want+"\nRun root "+tc.cmd.Name()+" --help for usage.\n", r.stderr, name)
		var argsErr *ArgsError
		require.ErrorAs(t, r.err, &argsErr, name)
		assert.Equal(t, "root "+tc.cmd.Name(), argsErr.Command, name)
	}
}

func TestArgsErrorCarriesWhatWasRefused(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(leafWith("get", Required("ID")))
	var argsErr *ArgsError
	require.ErrorAs(t, execTree(context.Background(), root, "get", "a", "b").err, &argsErr)
	assert.Equal(t, &ArgsError{Command: "root get", Unexpected: "b", Min: 1, Max: 1}, argsErr)
	require.ErrorAs(t, execTree(context.Background(), root, "get").err, &argsErr)
	assert.Equal(t, &ArgsError{Command: "root get", Missing: []string{"ID"}, Min: 1, Max: 1}, argsErr)
}

// A leaf with no declaration takes none, and a stray word is not an unknown
// command, which a leaf has none of.
func TestUndeclaredLeafTakesNoArguments(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "version", RunE: func(*cobra.Command, []string) error { return nil }})
	r := execTree(context.Background(), root, "version", "extra")
	require.Error(t, r.err)
	assert.Equal(t, "Error: unexpected argument \"extra\": root version takes no arguments\nRun root version --help for usage.\n", r.stderr)
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
}

func TestArgsAfterDash(t *testing.T) {
	build := func() (*cobra.Command, *[]string) {
		var got []string
		cmd := Args(&cobra.Command{Use: "add", RunE: func(_ *cobra.Command, args []string) error {
			got = args
			return nil
		}}, Optional("NAME"), Arg{Name: "COMMAND", Optional: true, Repeated: true, AfterDash: true})
		return cmd, &got
	}
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"nothing":                 {nil, ""},
		"a name":                  {[]string{"a"}, ""},
		"a name and a command":    {[]string{"a", "--", "tool", "--flag"}, ""},
		"a command alone":         {[]string{"--", "tool"}, ""},
		"two names":               {[]string{"a", "b"}, `unexpected argument "b": root add takes at most 1 argument`},
		"two names before a dash": {[]string{"a", "b", "--", "tool"}, `unexpected argument "b": root add takes at most 1 argument`},
	} {
		root := &cobra.Command{Use: "root"}
		cmd, _ := build()
		root.AddCommand(cmd)
		r := execTree(context.Background(), root, append([]string{"add"}, tc.args...)...)
		if tc.want == "" {
			assert.NoError(t, r.err, name)
		} else {
			assert.EqualError(t, r.err, tc.want, name)
		}
	}

	required := Args(&cobra.Command{Use: "run", RunE: func(*cobra.Command, []string) error { return nil }},
		Arg{Name: "COMMAND", Repeated: true, AfterDash: true})
	root := &cobra.Command{Use: "root"}
	root.AddCommand(required)
	assert.EqualError(t, execTree(context.Background(), root, "run").err, "root run requires COMMAND")
	assert.EqualError(t, execTree(context.Background(), root, "run", "--").err, "root run requires COMMAND")
	assert.NoError(t, execTree(context.Background(), root, "run", "--", "tool").err)
}

// A command with no dash argument counts the words after a -- as it always
// has: with the rest.
func TestDashMeansNothingToACommandWithoutADashArgument(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(leafWith("get", Optional("ID")))
	assert.NoError(t, execTree(context.Background(), root, "get", "--", "-x").err)
	assert.Error(t, execTree(context.Background(), root, "get", "--", "a", "b").err)
}

func TestCheckArgsRunsAfterTheCount(t *testing.T) {
	var seen [][]string
	cmd := CheckArgs(leafWith("create", Arg{Name: "KEY=VALUE", Optional: true, Repeated: true}), func(_ *cobra.Command, args []string) error {
		seen = append(seen, args)
		if len(args) > 0 && args[0] == "bad" {
			return Usage(errors.New("bad shape"))
		}
		return nil
	})
	root := &cobra.Command{Use: "root"}
	root.AddCommand(cmd)
	assert.NoError(t, execTree(context.Background(), root, "create", "a=b").err)
	assert.EqualError(t, execTree(context.Background(), root, "create", "bad").err, "bad shape")
	assert.Len(t, seen, 2)

	none := CheckArgs(leafWith("x", Required("A")), func(*cobra.Command, []string) error { t.Fatal("ran before the count"); return nil })
	root = &cobra.Command{Use: "root"}
	root.AddCommand(none)
	assert.EqualError(t, execTree(context.Background(), root, "x").err, "root x requires A")
	assert.Panics(t, func() { CheckArgs(&cobra.Command{Use: "y"}, nil) })
}

func TestCheckArgsBeforeSpeaksAheadOfTheCount(t *testing.T) {
	cmd := CheckArgsBefore(leafWith("logs", Optional("ID")), func(_ *cobra.Command, args []string) error {
		if len(args) > 0 && args[0] == "triggerer" {
			return Usage(errors.New(`"triggerer" names a component, not an ID`))
		}
		return nil
	})
	root := &cobra.Command{Use: "root"}
	root.AddCommand(cmd)
	assert.EqualError(t, execTree(context.Background(), root, "logs", "triggerer", "dep-1").err, `"triggerer" names a component, not an ID`)
	assert.EqualError(t, execTree(context.Background(), root, "logs", "dep-1", "dep-2").err, `unexpected argument "dep-2": root logs takes at most 1 argument`)
	assert.NoError(t, execTree(context.Background(), root, "logs", "dep-1").err)
}

func TestAnyWordsNeedsAReasonAndIsReadable(t *testing.T) {
	assert.Panics(t, func() { AnyWords(&cobra.Command{Use: "x"}, "") })
	cmd := AnyWords(&cobra.Command{Use: "x"}, "its RunE reads them")
	assert.Equal(t, "its RunE reads them", cmd.Annotations[AnyWordsAnnotation])
	assert.NoError(t, cmd.Args(cmd, []string{"any", "words"}))
	declared := AnyWords(leafWith("z", Required("A")), "it reads them now")
	assert.Equal(t, "z", declared.Use, "a declaration it had is gone with its Use text")
	assert.Empty(t, DeclaredArgs(declared))
}

func TestPassthroughStopsFlagParsingAtTheFirstWord(t *testing.T) {
	cmd := Passthrough(leafWith("run", Required("COMMAND"), Arg{Name: "ARGS", Optional: true, Repeated: true}))
	assert.Equal(t, "true", cmd.Annotations[ArgvPassthroughAnnotation])
	verbose := cmd.Flags().Bool("verbose", false, "")
	require.NoError(t, cmd.Flags().Parse([]string{"tool", "--verbose"}))
	assert.False(t, *verbose, "a flag after the first word is the wrapped command's")
	assert.Equal(t, []string{"tool", "--verbose"}, cmd.Flags().Args())
}
