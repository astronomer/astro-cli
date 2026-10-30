package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The CLI writes a command, flag or path in its output as plain text: "Use
// astro local start instead", not "Use `astro local start` instead". A
// terminal prints a backtick as it is, and a script or Astro Desktop reading
// an error object gets it as it is, so markdown in a message is noise wherever
// it lands. docs/*.md is markdown and keeps its backticks.
//
// backtickAllowed is what may hold a backtick anyway, because it is markdown
// or code where the backtick means something. A key is one of:
//   - a file, for one that writes such text throughout;
//   - a file and a declared name ("file: name"), for the literals that make up
//     one const or var;
//   - a file and one literal as written in it, for a single string.
//
// Each says why.
var backtickAllowed = map[string]string{
	"pkg/scaffold/templates.go: agentsContent": "the AGENTS.md astro init writes into a project is markdown",
	"pkg/scaffold/airflowupgradeprompt.go": "the prompt that hands an Airflow upgrade to Otto is markdown for a model, " +
		"and is never printed",
	"cmd/local/upgrade.go: cliUpgradePrompt": "the CLI's own words in the Otto upgrade prompt, which is markdown for a " +
		"model and is never printed",
	"pkg/scaffold/files1x.go: \"$*?`\\\\\"": "the shell metacharacters that make an argument unsafe to repeat unquoted",
	"pkg/scaffold/dockerfilerewrite.go: staleHelperHeader": "the first line of the install helper earlier previews wrote into a project, " +
		"matched byte for byte to tell the CLI's file from one of the project's own; never printed",
}

// Every string literal in the repo's non-test Go code is plain text, with no
// backtick, except what backtickAllowed names.
func TestMessagesWriteCommandsAsPlainText(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	used := map[string]bool{}
	for _, file := range trackedGoFiles(t, root) {
		if !heldToPlainText(file) {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file)), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		stringLiterals(f, "", func(lit *ast.BasicLit, decl string) {
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(s, "`") {
				return
			}
			keys := []string{file, file + ": " + lit.Value}
			if decl != "" {
				keys = append(keys, file+": "+decl)
			}
			for _, key := range keys {
				if _, ok := backtickAllowed[key]; ok {
					used[key] = true
					return
				}
			}
			problems = append(problems, file+": "+lit.Value)
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d string literals hold a backtick:\n\t%s\n\n"+
			"The CLI writes a command, flag or path as plain text, with no backticks "+
			"(\"Use astro local start instead\", not \"Use `astro local start` instead\"): a terminal "+
			"prints a backtick as it is, and a json consumer reads it. If the text is markdown or "+
			"code where a backtick means something, add it to backtickAllowed in "+
			"internal/archlint/backticks_test.go and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for key := range backtickAllowed {
		if !used[key] {
			t.Errorf("backtickAllowed names %s, which holds no backtick in a string any more: delete the entry", key)
		}
	}
}

// stringLiterals calls fn with every string literal under n and the name of
// the const or var it belongs to ("" outside one).
func stringLiterals(n ast.Node, decl string, fn func(lit *ast.BasicLit, decl string)) {
	ast.Inspect(n, func(n ast.Node) bool {
		if spec, ok := n.(*ast.ValueSpec); ok && len(spec.Names) > 0 {
			for _, v := range spec.Values {
				stringLiterals(v, spec.Names[0].Name, fn)
			}
			return false
		}
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			fn(lit, decl)
		}
		return true
	})
}

// heldToPlainText reports whether the rule covers a file: non-test Go code,
// outside the e2e module, the test-helper packages, generated API clients and
// test data.
func heldToPlainText(file string) bool {
	return !strings.HasSuffix(file, "_test.go") &&
		!strings.HasPrefix(file, "e2e/") &&
		!strings.HasPrefix(file, ".") &&
		!strings.HasSuffix(file, ".gen.go") &&
		!strings.Contains(file, "/testdata/") &&
		!strings.Contains(file, "cliouttest/") &&
		!strings.Contains(file, "instancestest/")
}

// trackedGoFiles lists the .go files git tracks under root, slash-separated
// and relative to it, so a worktree under .claude/, .git itself or an
// untracked scratch file is never read as the repo's code.
func trackedGoFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "--", "*.go")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		t.Fatal("git ls-files found no .go files")
	}
	return files
}
