package local

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// The conversion finds the value comparison by a type assertion, so a writer
// that stops implementing it silently falls back to treating every held value
// as different. init's writer has to keep answering it.
func TestInitsVaultWriterComparesValues(t *testing.T) {
	var w scaffold.SecretWriter = &lazyVaultWriter{dir: t.TempDir()}
	if _, ok := w.(scaffold.SecretValueChecker); !ok {
		t.Fatal("lazyVaultWriter does not implement scaffold.SecretValueChecker")
	}
}
