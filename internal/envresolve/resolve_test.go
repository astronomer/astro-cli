package envresolve

import (
	"errors"
	"reflect"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

const scope = "/Users/x/proj"

func vaultConn(t *testing.T, store secrets.Store, key string, c connmodel.Connection) {
	t.Helper()
	_, val, ok := airflowenv.EncodeConnEnv(c)
	if !ok {
		t.Fatalf("encode %+v", c)
	}
	if err := store.Set(key, val); err != nil {
		t.Fatal(err)
	}
}

func TestResolveLayering(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"FROM_ENV":    {Required: true}, // process env beats the vault
			"FROM_SCOPED": {Required: true}, // scoped vault beats global
			"FROM_GLOBAL": {Required: true},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Type: envschema.TypeInt, Required: true}, // via AIRFLOW_VAR_*
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true},
		},
	}
	store := secrets.NewMemoryStore()
	for key, val := range map[string]string{
		EnvVaultKey(scope, "FROM_ENV"):               "vault-loses",
		EnvVaultKey(scope, "FROM_SCOPED"):            "scoped-wins",
		EnvVaultKey("", "FROM_SCOPED"):               "global-loses",
		EnvVaultKey("", "FROM_GLOBAL"):               "global-wins",
		EnvVaultKey(scope, "AIRFLOW_VAR_BATCH_SIZE"): "100",
	} {
		if err := store.Set(key, val); err != nil {
			t.Fatal(err)
		}
	}
	vaultConn(t, store, ConnVaultKey(scope, "warehouse"),
		connmodel.Connection{ConnID: "warehouse", ConnType: "postgres", ConnPassword: "s3cret"}) //nolint:gosec // G101: test fixture, not a real credential

	res, err := Resolve(Inputs{
		Schema:  schema,
		Scope:   scope,
		Store:   store,
		Environ: []string{"FROM_ENV=env-wins", "UNDECLARED=ignored"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantValues := envschema.Values{
		EnvVars: map[string]string{
			"FROM_ENV":    "env-wins",
			"FROM_SCOPED": "scoped-wins",
			"FROM_GLOBAL": "global-wins",
		},
		AirflowVariables: map[string]string{"batch_size": "100"},
		Connections:      map[string]string{"warehouse": "postgres"},
	}
	if !reflect.DeepEqual(res.Values, wantValues) {
		t.Errorf("Values mismatch\n got: %+v\nwant: %+v", res.Values, wantValues)
	}
	if len(res.Violations) != 0 || len(res.Missing) != 0 {
		t.Errorf("want a clean result, got violations %v missing %v", res.Violations, res.Missing)
	}
	if res.VaultUnavailable {
		t.Error("VaultUnavailable = true with a store present")
	}
}

func TestResolveMissingReport(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"API_URL": {Type: envschema.TypeURL, Required: true, Description: "the upstream API"},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Required: true, Sensitive: true},
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true, Description: "the analytics DB"},
		},
	}
	res, err := Resolve(Inputs{Schema: schema, Scope: scope, Store: secrets.NewMemoryStore()})
	if err != nil {
		t.Fatal(err)
	}
	want := []Missing{
		{
			Section: envschema.SectionAirflowVariable, Name: "batch_size", Sensitive: true,
			EnvKey: "AIRFLOW_VAR_BATCH_SIZE", VaultKey: EnvVaultKey(scope, "AIRFLOW_VAR_BATCH_SIZE"),
		},
		{
			Section: envschema.SectionConnection, Name: "warehouse", Description: "the analytics DB",
			Sensitive: true, ConnType: "postgres",
			EnvKey: "AIRFLOW_CONN_WAREHOUSE", VaultKey: ConnVaultKey(scope, "warehouse"),
		},
		{
			Section: envschema.SectionEnvVar, Name: "API_URL", Description: "the upstream API",
			EnvKey: "API_URL", VaultKey: EnvVaultKey(scope, "API_URL"),
		},
	}
	if !reflect.DeepEqual(res.Missing, want) {
		t.Errorf("Missing mismatch\n got: %+v\nwant: %+v", res.Missing, want)
	}
	if len(res.Violations) != 3 {
		t.Errorf("want 3 violations, got %v", res.Violations)
	}
}

func TestResolveConnViolations(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres"},             // wrong type in the vault
			"api":       {ConnType: "http", Required: true}, // undecodable stored value
		},
	}
	store := secrets.NewMemoryStore()
	vaultConn(t, store, ConnVaultKey("", "warehouse"), connmodel.Connection{ConnID: "warehouse", ConnType: "mysql"})
	if err := store.Set(ConnVaultKey("", "api"), "not json"); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(Inputs{Schema: schema, Scope: scope, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	// The corrupt required conn reports exactly once — present-but-broken,
	// not missing — so the fix suggested is "repair the value".
	want := []envschema.Violation{
		{Kind: envschema.ViolationWrongType, Section: envschema.SectionConnection, Key: "api", Reason: "stored value is not valid connection JSON"},
		{Kind: envschema.ViolationWrongType, Section: envschema.SectionConnection, Key: "warehouse", Reason: `expected type "postgres", got "mysql"`},
	}
	if !reflect.DeepEqual(res.Violations, want) {
		t.Errorf("Violations mismatch\n got: %+v\nwant: %+v", res.Violations, want)
	}
	if len(res.Missing) != 0 {
		t.Errorf("corrupt value must not appear as missing, got %+v", res.Missing)
	}
}

func TestResolveConnFromProcessEnv(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ConnSpec{"warehouse": {ConnType: "postgres", Required: true}},
	}
	key, val, ok := airflowenv.EncodeConnEnv(connmodel.Connection{ConnID: "warehouse", ConnType: "postgres"})
	if !ok {
		t.Fatal("encode failed")
	}
	res, err := Resolve(Inputs{Schema: schema, Environ: []string{key + "=" + val}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Values.Connections["warehouse"]; got != "postgres" {
		t.Errorf("conn_type = %q, want postgres", got)
	}
	if !res.VaultUnavailable {
		t.Error("VaultUnavailable = false with a nil store")
	}
}

func TestResolveDeploymentBindingErrors(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"PROD_ONLY": {Bindings: map[string]envschema.Binding{
				envschema.EnvLocal: {Source: envschema.SourceDeployment, Deployment: "prod"},
			}},
		},
	}
	_, err := Resolve(Inputs{Schema: schema, Store: secrets.NewMemoryStore()})
	var dbe *DeploymentBindingError
	if !errors.As(err, &dbe) {
		t.Fatalf("want *DeploymentBindingError, got %v", err)
	}
	if dbe.Name != "PROD_ONLY" || dbe.Deployment != "prod" || dbe.Section != envschema.SectionEnvVar {
		t.Errorf("unexpected error detail: %+v", dbe)
	}
}

// TestResolveOtherEnvironmentBindingIgnored is the indirection working: a
// prod binding on a name changes nothing about a local resolve.
func TestResolveOtherEnvironmentBindingIgnored(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true, Bindings: map[string]envschema.Binding{
				"prod": {Source: envschema.SourceDeployment, Deployment: "prod"},
			}},
		},
	}
	store := secrets.NewMemoryStore()
	vaultConn(t, store, ConnVaultKey("", "warehouse"), connmodel.Connection{ConnID: "warehouse", ConnType: "postgres"})
	res, err := Resolve(Inputs{Schema: schema, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Violations) != 0 {
		t.Errorf("want clean result, got %v", res.Violations)
	}
}

func TestResolveNotLocal(t *testing.T) {
	_, err := Resolve(Inputs{Environment: "prod"})
	if !errors.Is(err, ErrNotLocal) {
		t.Fatalf("want ErrNotLocal, got %v", err)
	}
}

func TestResolveNilSchema(t *testing.T) {
	res, err := Resolve(Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Violations) != 0 || len(res.Missing) != 0 {
		t.Errorf("want empty result, got %+v", res)
	}
}
