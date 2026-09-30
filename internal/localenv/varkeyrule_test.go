package localenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `astro local env airflow-variable set` refuses a key with a leading digit,
// naming the rule, and writes nothing.
func TestSetRefusesAVariableKeyWithALeadingDigit(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	s := &Store{Path: path, Scope: ScopeProject}
	_, err := s.Set(KindVar, "1st", "v")
	if err == nil || !strings.Contains(err.Error(), `"1st" is not a valid variable key (letters, digits, _; no leading digit)`) {
		t.Fatalf("Set(1st) = %v, want the rule named", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("a refused set wrote %s", path)
	}
	if _, err := s.Set(KindVar, "_1st", "v"); err != nil {
		t.Errorf("Set(_1st) = %v, want it accepted", err)
	}
}
