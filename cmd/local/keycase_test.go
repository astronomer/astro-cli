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
//
// legacyKeys are the camelCase keys these payloads published before the rule
// held, and still do: renaming one now would break whoever reads it.
var legacyKeys = map[string]string{
	"api-endpoint-list.json: operationId":           "the OpenAPI field name, as published",
	"api-endpoint-list.json: pathParameters":        "published before the rule",
	"api-endpoint-row.json: operationId":            "the OpenAPI field name, as published",
	"api-endpoint-row.json: pathParameters":         "published before the rule",
	"init.json: airflowDefaultSource":               "published before the rule",
	"local-list-row.json: startedAt":                "published before the rule",
	"local-list.json: startedAt":                    "published before the rule",
	"local-removed.json: startedAt":                 "published before the rule",
	"local-status.json: airflowMajor":               "published before the rule",
	"local-status.json: projectPath":                "published before the rule",
	"local-status.json: startedAt":                  "published before the rule",
	"local-status.json: stopWithSession":            "published before the rule",
	"local-upgrade-airflow.json: coreReplaced":      "published before the rule",
	"local-upgrade-airflow.json: removedAirflowKey": "published before the rule",
	"local-upgrade-airflow.json: requiresPython":    "published before the rule",
	"local-upgrade-airflow.json: restartNeeded":     "published before the rule",
	"local-upgrade-airflow.json: runtimeRemoved":    "published before the rule",
}

func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0, legacyKeys),
		"add a json tag — snake_case — and run `make update-schemas`.")
}
