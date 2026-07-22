package envresolve

import (
	"reflect"
	"testing"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

func TestListing(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"API_URL": {Type: envschema.TypeURL, Required: true, Description: "the upstream API"},
			"TOKEN":   {Sensitive: true},
			"UNSET":   {},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Type: envschema.TypeInt},
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true},
		},
	}
	store := secrets.NewMemoryStore()
	for key, val := range map[string]string{
		EnvVaultKey(scope, "API_URL"):             "https://example.com", // project entry
		EnvVaultKey("", "API_URL"):                "https://global",      // shadowed by the project entry
		EnvVaultKey("", "TOKEN"):                  "t0k3n",
		EnvVaultKey("/elsewhere", "UNSET"):        "invisible here", // another project's scope
		EnvVaultKey("", "AIRFLOW_VAR_BATCH_SIZE"): "100",
		ConnVaultKey(scope, "warehouse"):          `{"conn_type":"postgres"}`,
		"auth:refresh-token":                      "outside the env namespace",
	} {
		if err := store.Set(key, val); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Listing(schema, store, scope)
	if err != nil {
		t.Fatal(err)
	}
	want := []ListedName{
		{Section: envschema.SectionAirflowVariable, Name: "batch_size", Type: envschema.TypeInt, InVault: true, VaultScope: VaultScopeGlobal},
		{Section: envschema.SectionConnection, Name: "warehouse", ConnType: "postgres", Required: true, Sensitive: true, InVault: true, VaultScope: VaultScopeProject},
		{Section: envschema.SectionEnvVar, Name: "API_URL", Type: envschema.TypeURL, Required: true, Description: "the upstream API", InVault: true, VaultScope: VaultScopeProject},
		{Section: envschema.SectionEnvVar, Name: "TOKEN", Sensitive: true, InVault: true, VaultScope: VaultScopeGlobal},
		{Section: envschema.SectionEnvVar, Name: "UNSET"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Listing mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestListingNilStore(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{"API_URL": {Required: true}},
	}
	got, err := Listing(schema, nil, scope)
	if err != nil {
		t.Fatal(err)
	}
	want := []ListedName{{Section: envschema.SectionEnvVar, Name: "API_URL", Required: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Listing = %+v, want %+v", got, want)
	}
}

func TestListingNilSchema(t *testing.T) {
	got, err := Listing(nil, secrets.NewMemoryStore(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Listing(nil schema) = %+v, want empty", got)
	}
}
