package runtimecatalog

import (
	"os"
	"path/filepath"
	"testing"
)

// A copy whose lines name no version (only "name @ url" or editable lines) is
// no freeze: the one reading of a freeze finds no pin in it, so it is never
// kept or used, and a seed reads the list again rather than taking it as
// "nothing to seed".
func TestAFreezeWithNoPinIsNotAFreeze(t *testing.T) {
	data := []byte("mylib @ https://example.invalid/mylib.whl\n-e ./local\n# comment\n")
	pins, err := freezePins(data)
	if err != nil || len(pins) != 0 {
		t.Fatalf("freezePins = %v, %v", pins, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runtime-3.3-9-python-3.12.txt"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := cachedFreeze(dir, "runtime-3.3-9-python-3.12.txt"); got != nil || err != nil {
		t.Errorf("a copy with no pin was used: %q, %v", got, err)
	}
}
