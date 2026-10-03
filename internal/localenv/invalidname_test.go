package localenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A .env entry an older build wrote under a leading-digit Variable key is
// listed with the reason, and delete removes it by its stored key.
func TestALeadingDigitDotenvVariableIsListedAndDeletable(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("AIRFLOW_VAR_1ST_REGION=us-east-1\nKEEP=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := orphanItem("AIRFLOW_VAR_1ST_REGION", "", true)
	if item.Name != "1st_region" || !strings.Contains(item.Invalid, "rename or delete it") || item.DeclareHint != "" || item.RemoveHint == "" {
		t.Errorf("row = %+v, want the reason, a remove hint and no declare hint", item)
	}

	s := &Store{Path: path, Scope: ScopeProject}
	ok, err := s.Delete(KindVar, "1st_region")
	if err != nil || !ok {
		t.Fatalf("Delete(1st_region) = %v, %v", ok, err)
	}
	if got := readFile(t, path); strings.Contains(got, "1ST_REGION") || !strings.Contains(got, "KEEP=1") {
		t.Errorf(".env = %q", got)
	}
	if _, err := s.Set(KindVar, "1st_region", "v"); err == nil {
		t.Error("set still refuses the name")
	}
}
