package local

import (
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// The `--output json` payloads are the contract this CLI publishes, and
// cliouttest is how they are pinned: each case in schema_cases_test.go is
// marshaled with every field populated and compared against its golden in
// testdata/schema. cmd/astro pins its own payloads the same way, and `make
// update-schemas` rewrites both.

// schemaDir holds this tree's goldens.
var schemaDir = filepath.Join("testdata", "schema")

// schemaCase is one published payload.
type schemaCase struct {
	// name is the golden file's stem, and reads as the surface it belongs
	// to rather than as the Go type, since the contract is the command's.
	name  string
	value any
}

func (c schemaCase) shared() cliouttest.Case {
	return cliouttest.Case{Name: c.name, Value: c.value}
}

// checkSchema marshals a fully-populated payload and holds it against the
// golden.
func checkSchema(t *testing.T, c schemaCase) {
	t.Helper()
	cliouttest.Check(t, schemaDir, c.shared())
}
