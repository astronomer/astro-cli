package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The CLI leads a warning, an error and a note with a label pkg/ansi defines and
// cmd/cliout passes on:
// Warn and WarningLabel (yellow), ErrorLabel (red), Note and NoteLabel
// (cyan), DangerLabel (red, for a confirmation of something that cannot be
// undone). Each colors its label on a terminal stdout or stderr. A message that
// spells "Warning: " into its own text prints it plain, in whatever case the
// author chose, and is the drift the sweep that added these labels removed.
//
// labelAllowed is what may start with such a label anyway, because it is not
// the CLI's own label. A key is a file, or a file and one literal as written
// in it (unquoted). Each says why.
var labelAllowed = map[string]string{
	"pkg/ansi/labels.go":                "defines the labels themselves, the one place their words are spelled",
	"pkg/container/command.go: Error: ": "parses the container engine's own output, which leads its errors with Error:",
	"pkg/uv/errors.go: error:":          "parses uv's own output, whose failure line starts error:",
	"internal/pack/pack.go: warning: ": "the fallback for a pack.Request with no Warn hook: the warning rides the build's " +
		"progress stream as a log line's text, which a progress reader shows as is, not as a terminal label",
}

// labelLiteral matches a string that holds a label the CLI prints through
// pkg/ansi: at its start or after any newline ("\nWarning: …", "done.\nNote:
// …"), after any spaces, in any case; or a string that is a label on its own
// ("Warning:"), handed to a print call as its own argument.
var labelLiteral = regexp.MustCompile(`(?i)(^|\n)[ \t]*(warning|error|note|next): |^\s*(warning|error|note|next):\s*$`)

// No non-test string literal holds a Warning, Error, Note or Next label,
// except what labelAllowed names. An allowlist key is a file, or a file and
// the literal's unquoted value.
func TestLabelsComeFromCliout(t *testing.T) {
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
		stringLiterals(f, "", func(lit *ast.BasicLit, _ string) {
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !labelLiteral.MatchString(s) {
				return
			}
			for _, key := range []string{file, file + ": " + s} {
				if _, ok := labelAllowed[key]; ok {
					used[key] = true
					return
				}
			}
			problems = append(problems, file+": "+lit.Value)
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d string literals start with a Warning, Error, Note or Next label:\n\t%s\n\n"+
			"Print the label through cmd/cliout (or pkg/ansi, from internal/) instead: Warn, Note, or WarningLabel, NextLabel, "+
			"ErrorLabel, NoteLabel, DangerLabel beside the message, so it is colored on a terminal and "+
			"spelled the same everywhere. If the text is not the CLI's own label, for example another "+
			"tool's output being parsed, add it to labelAllowed in internal/archlint/labels_test.go "+
			"and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for key := range labelAllowed {
		if !used[key] {
			t.Errorf("labelAllowed names %s, which no longer starts with a label: delete the entry", key)
		}
	}
}
