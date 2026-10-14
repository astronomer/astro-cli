package airflowrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCosmosBoostFile(t *testing.T) {
	for rel, want := range map[string]bool{
		".astro/dbt_metadata.json":                      true,
		".astro/manifest.slim.json":                     true,
		"dags/dbt/shop/target/.astro/dbt_metadata.json": true,
		"dbt_metadata.json":                             false,
		".astro/config.yaml":                            false,
		".astro/nested/dbt_metadata.json":               false,
		"x.astro/dbt_metadata.json":                     false,
	} {
		assert.Equal(t, want, IsCosmosBoostFile(rel), rel)
	}
}

// Each file is judged by its own marker: a sidecar by generated_by, a slim
// manifest by _generated_by, and anything else is the user's.
func TestCosmosBoostFileWrittenByAstro(t *testing.T) {
	const (
		sidecar = ".astro/dbt_metadata.json"
		slim    = ".astro/manifest.slim.json"
	)
	for _, tc := range []struct {
		rel, data string
		want      bool
	}{
		{sidecar, `{"generated_by":{"application":"astro","version":"1.0.0"}}`, true},
		{sidecar, `{"generated_by":{"application":"someone-else"}}`, false},
		{sidecar, `{"schema":1,"version":{"hash":"abc"}}`, false},
		{sidecar, `{"_generated_by":{"application":"astro"}}`, false},
		{sidecar, `not json`, false},
		{slim, `{"_generated_by":{"application":"astro"}}`, true},
		{slim, `{"generated_by":{"application":"astro"}}`, false},
		{slim, `{"_generated_by":{"application":"someone-else"}}`, false},
		{".astro/config.yaml", `{"generated_by":{"application":"astro"}}`, false},
		// The whole document is read: the marker after other keys, the last
		// of a repeated key, and nothing below the top level.
		{sidecar, `{"schema":1,"version":{"algo":"x","hash":"h"},"generated_by":{"application":"astro"}}`, true},
		{sidecar, `{"generated_by":{"application":"astro"},"generated_by":{"application":"someone-else"}}`, false},
		{slim, `{"nodes":{"a":{"_generated_by":{"application":"astro"}}}}`, false},
		// Malformed is not ours.
		{sidecar, `["generated_by",{"application":"astro"}]`, false},
		{sidecar, `{"generated_by":"astro"}`, false},
		{sidecar, `{"generated_by":{"application":"astro"}},`, false},
	} {
		assert.Equal(t, tc.want, CosmosBoostFileWrittenByAstro(tc.rel, []byte(tc.data)), "%s %s", tc.rel, tc.data)
	}
}
