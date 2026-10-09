package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// or code where the backtick means something. A key is a file, for one that
// writes such text throughout, or a file and one literal as written in it,
// for a single string. Each says why.
var backtickAllowed = map[string]string{
	"pkg/scaffold/templates.go": "the README and AGENTS.md astro init writes into a project are markdown",
	"pkg/scaffold/airflowupgradeprompt.go": "the prompt that hands an Airflow upgrade to Otto is markdown for a model, " +
		"and is never printed",
	"pkg/scaffold/files1x.go: \"$*?`\\\\\"": "the shell metacharacters that make an argument unsafe to repeat unquoted",
}

// Every string literal in the repo's non-test Go code is plain text, with no
// backtick, except what backtickAllowed names.
func TestMessagesWriteCommandsAsPlainText(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	used := map[string]bool{}
	goFiles(t, root, ".", func(rel string) {
		file := filepath.ToSlash(rel)
		if !heldToPlainText(file) {
			return
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(s, "`") {
				return true
			}
			for _, key := range []string{file, file + ": " + lit.Value} {
				if _, ok := backtickAllowed[key]; ok {
					used[key] = true
					return true
				}
			}
			problems = append(problems, file+": "+lit.Value)
			return true
		})
	})
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
