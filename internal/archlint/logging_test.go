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

// The diagnostic log (pkg/logger, and log/slog in the sub-modules) is for
// detail --verbosity turns on, at debug. Something the person running a
// command needs to know is a warning, written with cliout.Warn or ansi.Warn,
// which always prints: a log line at warn could be hidden by --verbosity
// error, which is how a --page-size cut to 100 went unsaid
// (docs/architecture.md, Logging).
//
// A log line above debug belongs only where no person is reading the
// command's stderr: a background process with a log file of its own, or a
// full-screen program that sends the CLI's log to a file. logAboveDebugAllowed
// names those files, each with why.
var logAboveDebugAllowed = map[string]string{
	"internal/otto/warehouse.go": "runs while otto's full-screen UI owns the terminal; the CLI's log goes to " +
		"otto's log file (redirectCLILogs), raised to info so these lines are kept",
	"pkg/proxy/theme.go":       "runs in the proxy daemon, which has no terminal; its log is the daemon's log file",
	"pkg/proxy/airflowauth.go": "runs in the proxy daemon, which has no terminal; its log is the daemon's log file",
}

// logAboveDebug is, per import path, the package-level functions that write a
// log line above debug. The standard log package's Print family is included:
// slog's default carries it, at info.
var logAboveDebug = map[string]map[string]bool{
	modulePrefix + "pkg/logger": {"Infof": true, "Warnf": true},
	"log/slog": {
		"Info": true, "Warn": true, "Error": true,
		"InfoContext": true, "WarnContext": true, "ErrorContext": true,
	},
	"log": {"Print": true, "Printf": true, "Println": true},
}

func TestTheLogStaysAtDebugWhereSomeoneReadsStderr(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	used := map[string]bool{}
	for _, file := range trackedGoFiles(t, root) {
		if !heldToPlainText(file) || strings.HasPrefix(file, "pkg/logger/") {
			continue
		}
		calls := logCallsAboveDebug(t, filepath.Join(root, filepath.FromSlash(file)))
		if len(calls) == 0 {
			continue
		}
		if _, ok := logAboveDebugAllowed[file]; ok {
			used[file] = true
			continue
		}
		for _, c := range calls {
			problems = append(problems, file+": "+c)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d calls write a log line above debug:\n\t%s\n\n"+
			"Something the person running the command should know is a warning: write it with "+
			"cliout.Warn (ansi.Warn below cmd/), which always prints. Detail for when something goes "+
			"wrong is a debug line (logger.Debugf, slog.Debug). If this code runs where nobody reads "+
			"the command's stderr (a daemon, a full-screen program), add the file to "+
			"logAboveDebugAllowed in internal/archlint/logging_test.go and say why.",
			len(problems), strings.Join(problems, "\n\t"))
	}
	for file := range logAboveDebugAllowed {
		if !used[file] {
			t.Errorf("logAboveDebugAllowed names %s, which logs nothing above debug any more: delete the entry", file)
		}
	}
}

// logCallsAboveDebug is the calls in the file at path that write a log line
// above debug (logAboveDebug), as the file spells them.
func logCallsAboveDebug(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{} // the name the file uses for each watched import
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || logAboveDebug[p] == nil {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = p
	}
	if len(names) == 0 {
		return nil
	}
	var calls []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && logAboveDebug[names[pkg.Name]][sel.Sel.Name] {
			calls = append(calls, pkg.Name+"."+sel.Sel.Name)
		}
		return true
	})
	return calls
}
