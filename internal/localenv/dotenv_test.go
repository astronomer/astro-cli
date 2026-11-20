package localenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// Every value a set writes reads back unchanged, including the ones the
// parser's quoting rules make awkward: both quote kinds, a backslash before
// where a closing quote would go, escapes that a double-quoted value would
// otherwise expand.
func TestFormatValueRoundTripsTheHardCases(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	for _, v := range []string{
		`both 'single' and "double"`,
		`ends in a backslash\`,
		`it's \n not a newline`,
		`{"conn_type": "http", "extra": {"k": "a\\nb"}}`,
		`$HOME stays`,
		`a #not a comment`,
		`'starts with a quote`,
	} {
		if _, err := s.Set(KindEnv, "K", v); err != nil {
			t.Fatalf("set %q: %v", v, err)
		}
		got, ok, err := s.Get(KindEnv, "K")
		if err != nil || !ok {
			t.Fatalf("get after set %q: ok=%v err=%v", v, ok, err)
		}
		if got != v {
			t.Fatalf("round trip: got %q, want %q (file %q)", got, v, readFile(t, s.Path))
		}
	}
}

// A set or delete of a key whose quoted value spans lines edits the whole
// assignment, so no continuation line is left behind to be read as something
// else.
func TestMergeEditsAMultiLineAssignmentWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "A=1\nK='line one\nB=not a key\n'\nC=3\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path, Scope: ScopeProject}
	if _, err := s.Set(KindEnv, "K", "short"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), "A=1\nK=short\nC=3\n"; got != want {
		t.Fatalf("after set:\n got: %q\nwant: %q", got, want)
	}

	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Delete(KindEnv, "K"); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	if got, want := readFile(t, path), "A=1\nC=3\n"; got != want {
		t.Fatalf("after delete:\n got: %q\nwant: %q", got, want)
	}
}

// A set replaces the assignment that wins, the last, so the value it wrote is
// the one a read (and a start) sees.
func TestMergeSetReplacesTheWinningDuplicate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("K=first\nK=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path, Scope: ScopeProject}
	if _, err := s.Set(KindEnv, "K", "new"); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Get(KindEnv, "K")
	if err != nil || got != "new" {
		t.Fatalf("get after set = %q, %v; file %q", got, err, readFile(t, path))
	}
}

func TestDotenvWarnings(t *testing.T) {
	dir := t.TempDir()
	content := "" +
		"not an assignment\n" +
		// Undeclared and broken: Airflow would call it not defined.
		`AIRFLOW_CONN_BROKEN='{"conn_type": "http",'` + "\n" +
		// Declared and broken: the resolver's warning, not this one.
		`AIRFLOW_CONN_DECLARED='{"conn_type": "http",'` + "\n" +
		// A URI is not JSON and is not checked.
		"AIRFLOW_CONN_URI=http://host\n" +
		`AIRFLOW_CONN_GOOD='{"conn_type": "http"}'` + "\n"
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := LoadSources(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := envschema.ParseSchema(map[string]any{"connections": map[string]any{"declared": map[string]any{}}})
	if err != nil {
		t.Fatal(err)
	}
	got := src.DotenvWarnings(schema)
	if len(got) != 2 {
		t.Fatalf("want 2 warnings, got %d: %+v", len(got), got)
	}
	if got[0].Section != SectionDotenv || got[0].Key != "line 1" {
		t.Errorf("first warning = %+v, want the unreadable line 1", got[0])
	}
	if got[1].Section != envschema.SectionConnection || got[1].Key != "broken" {
		t.Errorf("second warning = %+v, want connection broken", got[1])
	}
}

// A set whose assignment an earlier open quote would take in is refused, and
// the file is left as it was, rather than reported done and never read back.
func TestMergeSetRefusesWhatWouldNotReadBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "A=\"oops\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path, Scope: ScopeProject}
	if _, err := s.Set(KindEnv, "B", `{"a":1}`); err == nil {
		t.Fatalf("set should be refused; file now %q", readFile(t, path))
	}
	if got := readFile(t, path); got != original {
		t.Fatalf("a refused set changed the file: %q", got)
	}
}

// A line the parser kept, minus text after its closing quote, is not reported
// as one Airflow never gets.
func TestDotenvWarningsSayNotPassedOnlyForSkippedLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte("K=\"v\" extra\njunk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := LoadSources(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := src.DotenvWarnings(nil)
	if len(got) != 2 {
		t.Fatalf("want 2 warnings, got %+v", got)
	}
	if strings.Contains(got[0].Reason, "not passed") {
		t.Errorf("kept line 1 reported as not passed: %q", got[0].Reason)
	}
	if !strings.Contains(got[1].Reason, "not passed") {
		t.Errorf("skipped line 2 should say it is not passed: %q", got[1].Reason)
	}
}

// A value holding a '$' is never written bare, where a reader that
// interpolates (Compose, python-dotenv) would expand it: single-quoted, or
// with the dollar escaped when it has to be double-quoted. Either way it reads
// back unchanged.
func TestSetQuotesADollar(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	for v, line := range map[string]string{
		"pa$word":     `K='pa$word'`,
		"${HOME}":     `K='${HOME}'`,
		"it's $5":     `K="it's \$5"`,
		`ends $here\`: `K="ends \$here\\"`,
		"no dollar":   `K='no dollar'`,
		"plain":       `K=plain`,
		"$":           `K='$'`,
		`a\$b`:        `K='a\$b'`,
		`say "hi" $x`: `K='say "hi" $x'`,
		`it's a\$b`:   `K="it's a\\\$b"`,
	} {
		if _, err := s.Set(KindEnv, "K", v); err != nil {
			t.Fatalf("set %q: %v", v, err)
		}
		if got := strings.TrimSuffix(readFile(t, s.Path), "\n"); got != line {
			t.Errorf("set %q wrote %s, want %s", v, got, line)
		}
		if got, _, err := s.Get(KindEnv, "K"); err != nil || got != v {
			t.Errorf("set %q read back %q (err %v)", v, got, err)
		}
	}
}
