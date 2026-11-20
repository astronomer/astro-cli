package envresolve

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// secretProvider is a test provider that says which of its values are
// secrets (Secrecy), as a vault tier or the Workspace does.
type secretProvider struct {
	mapProvider
	secret map[string]bool
}

func (p secretProvider) Secret(key string) bool { return p.secret[key] }

// A value from a source that holds it as a secret is not quoted back in a
// wrong-type reason, though the declaration leaves out `secret = true`: the
// reason goes to the terminal, the JSON event stream and CI logs. The same
// mistake with a plain value still says what the value was, and the schema
// the caller passed is left as written.
func TestResolveKeepsASecretSourcesValueOutOfTypeWarnings(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"VAULT_TOKEN": {Type: envschema.TypeInt},
			"CLOUD_TOKEN": {Type: envschema.TypeInt, Source: envschema.SourceWorkspace},
			"PORT":        {Type: envschema.TypePort},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"api_key": {Type: envschema.TypeInt},
		},
	}
	shell := mapProvider{label: "shell", vals: map[string]string{"PORT": "not-a-port"}}
	vault := secretProvider{
		mapProvider: mapProvider{label: "project vault", vals: map[string]string{
			"VAULT_TOKEN":         "sk-vault-123",
			"AIRFLOW_VAR_API_KEY": "sk-var-456",
		}},
		secret: map[string]bool{"VAULT_TOKEN": true, "AIRFLOW_VAR_API_KEY": true},
	}
	workspace := secretProvider{
		mapProvider: mapProvider{label: "workspace", vals: map[string]string{"CLOUD_TOKEN": "sk-cloud-789"}},
		secret:      map[string]bool{"CLOUD_TOKEN": true},
	}

	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{shell, vault}, WorkspaceProvider: workspace})
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, v := range res.Violations {
		if v.Kind == envschema.ViolationWrongType {
			reasons[v.Key] = v.Reason
		}
	}
	for _, key := range []string{"VAULT_TOKEN", "CLOUD_TOKEN", "api_key"} {
		got, ok := reasons[key]
		if !ok || !strings.HasPrefix(got, "expected an integer") {
			t.Errorf("%s: reason %q, want a wrong-type reason", key, got)
		}
		if strings.Contains(got, "sk-") {
			t.Errorf("%s: the reason quotes a secret source's value: %q", key, got)
		}
	}
	if got := reasons["PORT"]; !strings.Contains(got, `"not-a-port"`) {
		t.Errorf("PORT: reason %q, want the plain value quoted back", got)
	}
	if schema.EnvVars["VAULT_TOKEN"].Secret || schema.AirflowVariables["api_key"].Secret {
		t.Error("Resolve changed the caller's schema")
	}
}
