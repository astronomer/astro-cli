package archlint

import (
	"go/parser"
	"go/token"
	"testing"
)

// What the TestMain reader sees in a few shapes of TestMain.
func TestTestMainCallReader(t *testing.T) {
	const head = "package p\nimport (\n\t\"testing\"\n\t\"github.com/astronomer/astro-cli/pkg/testenv\"\n)\n"
	for name, tc := range map[string]struct {
		body           string
		main, isolates bool
		hooks          bool
		from           string
	}{
		"plain": {
			body: "func TestMain(m *testing.M) { testenv.Main(m) }",
			main: true, isolates: true, from: testenvPath,
		},
		"both hooks": {
			body: "func TestMain(m *testing.M) { testenv.Main(m, testenv.Options{MockKeyring: a, KeyringRefusals: b}) }",
			main: true, isolates: true, hooks: true, from: testenvPath,
		},
		"a nil hook is no hook": {
			body: "func TestMain(m *testing.M) { testenv.Main(m, testenv.Options{MockKeyring: nil, KeyringRefusals: b}) }",
			main: true, isolates: true, hooks: false, from: testenvPath,
		},
		"one hook": {
			body: "func TestMain(m *testing.M) { testenv.Main(m, testenv.Options{MockKeyring: a}) }",
			main: true, isolates: true, hooks: false, from: testenvPath,
		},
		"no TestMain": {body: "func helper() {}"},
		"a TestMain that never isolates": {
			body: "func TestMain(m *testing.M) { m.Run() }",
			main: true,
		},
		"a bare call is reported as bare, for the caller to place": {
			body: "func TestMain(m *testing.M) { Main(m) }",
			main: true, isolates: true, from: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "x_test.go", head+tc.body, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			main, call := testMainCall(f)
			if main != tc.main || (call != nil) != tc.isolates {
				t.Fatalf("main=%v isolates=%v, want %v %v", main, call != nil, tc.main, tc.isolates)
			}
			if call != nil && (call.hooks != tc.hooks || call.from != tc.from) {
				t.Errorf("hooks=%v from=%q, want %v %q", call.hooks, call.from, tc.hooks, tc.from)
			}
		})
	}
}

// A bare Main or Isolate counts only inside testenv itself.
func TestOnlyTestenvMayCallItBare(t *testing.T) {
	for _, tc := range []struct {
		root, dir string
		want      bool
	}{
		{"/repo", "pkg/testing/testenv", true},
		{"/repo/pkg/testenv", ".", true},
		{"/repo", "internal/otto", false},
		{"/repo/pkg/uv", ".", false},
		{"/repo/other/testenv", ".", false},
	} {
		if got := isTestenvDir(tc.root, tc.dir); got != tc.want {
			t.Errorf("isTestenvDir(%q, %q) = %v, want %v", tc.root, tc.dir, got, tc.want)
		}
	}
}
