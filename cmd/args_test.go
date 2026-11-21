package cmd

import (
	"errors"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// A command's positionals are declared once, with cliout.Args, and everything
// that reads them (the Use line, the argument check, the JSON help) reads that
// declaration. These tests hold every command in every tree to it.

// TestEveryCommandDeclaresItsPositionalsOrTakesNone walks every tree. A command
// has no positional in its Use line that it did not declare, and a command that
// declares none takes none: a stray word is refused as a usage error before any
// pre-run, so a typo is told apart from a command that was run right.
func TestEveryCommandDeclaresItsPositionalsOrTakesNone(t *testing.T) {
	var problems []string
	for _, tree := range rootsUnderTest(t) {
		// Cobra adds the help command as it executes; execute adds it first.
		tree.root.InitDefaultHelpCmd()
		walkCmd(tree.root, func(c *cobra.Command) {
			path := c.CommandPath()
			want := c.Name()
			if declared := cliout.DeclaredArgs(c); len(declared) > 0 {
				want += " " + useLineOf(declared)
			}
			if c.Use != want {
				problems = append(problems, "["+tree.name+"] "+path+": Use is "+quote(c.Use)+", the declaration shows "+quote(want))
			}
		})
	}
	sort.Strings(problems)
	assert.Empty(t, problems, "declare positionals with cliout.Args, and take none by declaring none")
}

func quote(s string) string { return `"` + s + `"` }

// useLineOf is what a Use line shows for declared, written out here rather than
// read from cliout, so that this test is a second opinion on it.
func useLineOf(declared []cliout.Arg) string {
	words := []string{}
	for _, a := range declared {
		dots := map[bool]string{true: "...", false: ""}[a.Repeated]
		switch {
		case a.AfterDash && a.Optional:
			words = append(words, "[-- "+a.Name+dots+"]")
		case a.AfterDash:
			words = append(words, "-- <"+a.Name+">"+dots)
		case a.Optional:
			words = append(words, "["+a.Name+dots+"]")
		default:
			words = append(words, "<"+a.Name+">"+dots)
		}
	}
	return strings.Join(words, " ")
}

// rawArgsAllowed are the files that may touch a command's Args themselves,
// each with why: they are what implements the declaration.
var rawArgsAllowed = map[string]string{
	"cliout/args.go":           "the declaration itself: writes Args, composes a check after the count, and accepts any words",
	"cliout/execute.go":        "wraps every command's check so what it refuses is a usage error",
	"cliout/deprecated.go":     "wraps a deprecated command's check with its warning, keeping the check it had",
	"utils/preferred_flags.go": "wraps a tree's checks with the hook that reconciles flags before the count is read, keeping them",
}

// rawArgsNames are cobra's own argument validators, which count words and say
// so in cobra's English instead of the CLI's.
var rawArgsNames = map[string]bool{
	"NoArgs": true, "ArbitraryArgs": true, "ExactArgs": true, "MaximumNArgs": true,
	"MinimumNArgs": true, "RangeArgs": true, "OnlyValidArgs": true, "MatchAll": true,
}

// TestNoCommandSetsItsOwnArgsValidator reads the sources of every command: a
// cobra.*Args validator, an Args field or an assignment to Args outside
// cliout.Args is a declaration that nothing ties to the Use line.
func TestNoCommandSetsItsOwnArgsValidator(t *testing.T) {
	var problems []string
	fset := gotoken.NewFileSet()
	require.NoError(t, filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		if _, ok := rawArgsAllowed[filepath.ToSlash(path)]; ok {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		problems = append(problems, rawArgsIn(fset, f)...)
		return nil
	}))
	sort.Strings(problems)
	assert.Empty(t, problems, "declare positionals with cliout.Args (cliout.CheckArgs for a check of the words, cliout.AnyWords for a command that reads them itself)")
	for file, why := range rawArgsAllowed {
		assert.NotEmpty(t, why, "rawArgsAllowed[%q] has no reason", file)
		assert.FileExists(t, filepath.FromSlash(file))
	}
}

// rawArgsIn is where f names a cobra validator, gives a cobra.Command literal
// an Args field, or assigns to an Args.
func rawArgsIn(fset *gotoken.FileSet, f *ast.File) []string {
	var found []string
	at := func(p gotoken.Pos, what string) { found = append(found, fset.Position(p).String()+": "+what) }
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && id.Name == "cobra" && rawArgsNames[n.Sel.Name] {
				at(n.Pos(), "cobra."+n.Sel.Name)
			}
		case *ast.CompositeLit:
			if hasArgsField(n) {
				at(n.Pos(), "a cobra.Command with an Args field")
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Args" {
					at(n.Pos(), "an assignment to Args")
				}
			}
		}
		return true
	})
	return found
}

// hasArgsField is whether lit is a cobra.Command literal that sets Args.
func hasArgsField(lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Command" {
		return false
	}
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Args" {
				return true
			}
		}
	}
	return false
}

// TestHelpJSONArgumentsEqualTheDeclaration holds the JSON help of every
// command in every tree to its declaration.
func TestHelpJSONArgumentsEqualTheDeclaration(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		tree.root.InitDefaultHelpCmd()
		walkCmd(tree.root, func(c *cobra.Command) {
			want := []helpArg{}
			for _, a := range cliout.DeclaredArgs(c) {
				want = append(want, helpArg{Name: a.Name, Required: !a.Optional, Repeated: a.Repeated})
			}
			assert.Equal(t, want, describeCommand(c).Arguments, "[%s] %s", tree.name, c.CommandPath())
		})
	}
}

// TestDeclaredCountsAreWhatTheCheckEnforces probes every declared command with
// every number of words from none to one past what it takes: the declared
// count check refuses exactly the lines outside it, as an ArgsError, and a
// check of the words themselves may refuse the rest only as something else.
func TestDeclaredCountsAreWhatTheCheckEnforces(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		tree.root.InitDefaultHelpCmd()
		walkCmd(tree.root, func(c *cobra.Command) {
			declared := cliout.DeclaredArgs(c)
			if len(declared) == 0 || c.Args == nil {
				return
			}
			minimum, maximum := wordsBeforeDash(declared)
			limit := maximum + 1
			if maximum < 0 {
				limit = minimum + 3
			}
			for n := 0; n <= limit; n++ {
				err := probe(c, strings.Fields(strings.Repeat("a ", n)))
				var argsErr *cliout.ArgsError
				inside := n >= minimum && (maximum < 0 || n <= maximum)
				assert.Equal(t, !inside, errors.As(err, &argsErr), "[%s] %s with %d words: %v", tree.name, c.CommandPath(), n, err)
			}
		})
	}
}

// wordsBeforeDash is how few and how many words a line takes before any --
// (-1 for no limit), which is all a line with no dash has.
func wordsBeforeDash(declared []cliout.Arg) (minimum, maximum int) {
	for _, a := range declared {
		if a.AfterDash {
			continue
		}
		if !a.Optional {
			minimum++
		}
		if maximum >= 0 {
			maximum++
		}
		if a.Repeated {
			maximum = -1
		}
	}
	return minimum, maximum
}

// probe runs c's check on words, as a run would before any pre-run. A check
// that reads flags a run has parsed is allowed to give up (the panic is its
// own, not the count's).
func probe(c *cobra.Command, words []string) (err error) {
	defer func() {
		if recover() != nil {
			err = nil
		}
	}()
	return c.Args(c, words)
}

// The wording of a stray and of a missing argument, on real commands: none, one
// optional, one required, a repeated one, and more than one required.
func TestArgumentRefusalsOnRealCommands(t *testing.T) {
	for _, tc := range []struct {
		command string
		args    []string
		want    string
	}{
		{"version", []string{"extra"}, `unexpected argument "extra": astro version takes no arguments`},
		{"deployment delete", []string{"a", "b"}, `unexpected argument "b": astro deployment delete takes at most 1 argument`},
		{"config get", nil, `astro config get requires SETTING_NAME`},
		{"config get", []string{"a", "b"}, `unexpected argument "b": astro config get takes exactly 1 argument`},
		{"config set", nil, `astro config set requires SETTING_NAME and VALUE`},
		{"config set", []string{"a"}, `astro config set requires VALUE`},
		{"config set", []string{"a", "b", "c"}, `unexpected argument "c": astro config set takes exactly 2 arguments`},
		{"af tasks clear", nil, `astro af tasks clear requires DAG_ID, RUN_ID and TASK_ID`},
		{"af tasks clear", []string{"d", "r"}, `astro af tasks clear requires TASK_ID`},
		{"local env variable link", nil, `astro local env variable link requires NAME`},
	} {
		line := append(strings.Fields(tc.command), tc.args...)
		run := runLine(t, line...)
		assert.Equal(t, 2, run.code, line)
		assert.Contains(t, run.stderr, "Error: "+tc.want+"\n", line)
		assert.Contains(t, run.stderr, "Run astro "+tc.command+" --help for usage.", line)
		assert.False(t, run.reachedLogin, line)
	}
}
