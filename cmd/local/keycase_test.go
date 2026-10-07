package local

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// A published key that is not snake_case is usually a Go field name that
// reached the wire because somebody forgot a tag.
//
// It is never a choice. encoding/json falls back to the field's own name, so
// `Section string` publishes "Section" while the tagged field beside it
// publishes "source_note" — which is how start-missing-env came to mix the
// two, in the payload whose own doc says it is structured for a coding agent
// to act on. Nothing noticed until the shapes were pinned and somebody read
// the file. cliouttest.KeyProblems checks the class, for every tree.
func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0, nil),
		"add a json tag — snake_case — and run `make update-schemas`.")
}
