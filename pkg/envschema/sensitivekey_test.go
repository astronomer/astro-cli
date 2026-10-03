package envschema

import (
	"errors"
	"strings"
	"testing"
)

// The key's earlier name is refused, and the refusal names the one to use, so
// a manifest written before the rename says how to fix itself. Exactly one
// problem: on a connection it must not also be judged as a secret key.
func TestParseSchemaRefusesSensitiveWithAHint(t *testing.T) {
	for _, tc := range []struct{ name, body, key string }{
		{"env var", "[tool.astro.env]\nTOKEN = { sensitive = true }", "tool.astro.env.TOKEN.sensitive"},
		{"connection", "[tool.astro.env.connections]\nwarehouse = { conn_type = 'postgres', sensitive = false }", "tool.astro.env.connections.warehouse.sensitive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema(decodeEnv(t, tc.body))
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if len(se.Problems) != 1 {
				t.Fatalf("want exactly one problem, got %+v", se.Problems)
			}
			pr := se.Problems[0]
			if pr.Key != tc.key || pr.Code != CodeUnknownField || !strings.Contains(pr.Reason, "use secret") {
				t.Errorf("want unknown_field at %s naming secret, got %+v", tc.key, pr)
			}
		})
	}
}
