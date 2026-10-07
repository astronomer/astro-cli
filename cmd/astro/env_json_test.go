package astro

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// envPublished is every type `astro env` publishes under -o json.
var envPublished = []any{
	env.InventoryList{},       // env list
	env.VariableList{},        // variable list
	env.ConnectionList{},      // connection list
	env.AirflowVariableList{}, // airflow-variable list
	env.MetricsExportList{},   // metrics-export list
	env.ObjectInfo{},          // get, every kind
	env.VarLinksReport{},      // variable link list
	env.LinksReport{},         // connection and airflow-variable link list
	env.SetFromFileResult{},   // variable and airflow-variable set --from-file
}

// TestEnvJSONKeysAreSnakeCase walks the json tags of every type `astro env`
// publishes, nested types included, and fails on a key that is not
// snake_case. Until 2.0 these published the Astro API's own models, whose
// keys are camelCase; a field added with a camelCase tag, or a type that goes
// back to carrying a generated API model, fails here.
func TestEnvJSONKeysAreSnakeCase(t *testing.T) {
	apiPkg := reflect.TypeOf(astrov1.EnvironmentObject{}).PkgPath()
	for _, v := range envPublished {
		typ := reflect.TypeOf(v)
		t.Run(typ.String(), func(t *testing.T) {
			keys := jsonKeys(typ, typ.Name(), map[reflect.Type]bool{})
			require.NotEmpty(t, keys)
			for _, key := range keys {
				name := key[strings.LastIndex(key, ".")+1:]
				assert.Regexp(t, snakeCaseKey, name, "key %s", key)
			}
			for _, st := range structTypes(typ, map[reflect.Type]bool{}) {
				assert.NotEqual(t, apiPkg, st.PkgPath(), "%s publishes the API's own %s", typ, st)
			}
		})
	}
}

// structTypes returns every struct type a value of typ can publish.
func structTypes(typ reflect.Type, seen map[reflect.Type]bool) []reflect.Type {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true
	out := []reflect.Type{typ}
	for i := range typ.NumField() {
		out = append(out, structTypes(typ.Field(i).Type, seen)...)
	}
	return out
}

// mockEnvList answers the one list call a read makes with objs, checking
// that it asked the platform for secrets exactly when the command was told to.
func mockEnvList(t *testing.T, showSecrets bool, objs ...astrov1.EnvironmentObject) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p.ShowSecrets != nil && *p.ShowSecrets == showSecrets
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: objs, TotalCount: len(objs)},
	}, nil).Once()
	return mc
}

// decodeObject decodes one published object, keeping each value raw so a
// test can ask both what a key holds and whether it is there at all.
func decodeObject(t *testing.T, out string) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	return got
}

// A secret variable's value is published as the platform returned it: blank
// unless --include-secrets asked for it, and then the value. is_secret says
// which it is either way, and set_fields that a blank secret is set.
func TestEnvVarGetJSONPublishesSecretsOnlyWhenAsked(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags []string
		value string
	}{
		{"without --include-secrets", nil, ""},
		{"with --include-secrets", []string{"--include-secrets"}, "shh"},
	} {
		t.Run(c.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			mc := mockEnvList(t, c.value != "", astrov1.EnvironmentObject{
				ObjectKey:           "TOKEN",
				ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
				Scope:               astrov1.EnvironmentObjectScopeWORKSPACE,
				ScopeEntityId:       "ws-test",
				SetFields:           []string{"value"},
				EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{IsSecret: true, Value: c.value},
			})
			astroV1Client = mc

			out, err := execEnvCmd(append([]string{"variable", "get", "TOKEN", "--workspace-id", "ws-test", "-o", "json"}, c.flags...)...)
			require.NoError(t, err)
			var got struct {
				ObjectKey           string   `json:"object_key"`
				ObjectType          string   `json:"object_type"`
				Scope               string   `json:"scope"`
				ScopeEntityID       string   `json:"scope_entity_id"`
				SetFields           []string `json:"set_fields"`
				EnvironmentVariable struct {
					IsSecret bool   `json:"is_secret"`
					Value    string `json:"value"`
				} `json:"environment_variable"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &got), out)
			assert.Equal(t, "TOKEN", got.ObjectKey)
			assert.Equal(t, "ENVIRONMENT_VARIABLE", got.ObjectType)
			assert.Equal(t, "WORKSPACE", got.Scope)
			assert.Equal(t, "ws-test", got.ScopeEntityID)
			assert.Equal(t, []string{"value"}, got.SetFields)
			assert.True(t, got.EnvironmentVariable.IsSecret)
			assert.Equal(t, c.value, got.EnvironmentVariable.Value)
			mc.AssertExpectations(t)
		})
	}
}

// A connection's password is there when the platform returned it and absent
// when it did not, as are the object's optional fields: an absent field is
// not published as null or "". A list publishes the same object under its key.
func TestEnvConnListJSONKeepsAbsentFieldsAbsent(t *testing.T) {
	host, password, port := "db.prod", "hunter2", 5432
	for _, c := range []struct {
		name     string
		flags    []string
		password *string
	}{
		{"without --include-secrets", nil, nil},
		{"with --include-secrets", []string{"--include-secrets"}, &password},
	} {
		t.Run(c.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			noLinks := []astrov1.EnvironmentObjectLink{}
			mc := mockEnvList(t, c.password != nil, astrov1.EnvironmentObject{
				ObjectKey:  "warehouse",
				ObjectType: astrov1.EnvironmentObjectObjectTypeCONNECTION,
				Scope:      astrov1.EnvironmentObjectScopeWORKSPACE,
				SetFields:  []string{"host", "password", "port"},
				Links:      &noLinks,
				Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: &host, Port: &port, Password: c.password},
			})
			astroV1Client = mc

			out, err := execEnvCmd(append([]string{"connection", "list", "--workspace-id", "ws-test", "-o", "json"}, c.flags...)...)
			require.NoError(t, err)
			var list struct {
				Connections []json.RawMessage `json:"connections"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &list), out)
			require.Len(t, list.Connections, 1)
			obj := decodeObject(t, string(list.Connections[0]))

			for _, absent := range []string{"id", "description", "auto_link_deployments", "exclude_links", "created_at", "created_by", "environment_variable", "metrics_export"} {
				assert.NotContains(t, obj, absent)
			}
			assert.JSONEq(t, `[]`, string(obj["links"]), "an empty list the API returned stays []")

			conn := decodeObject(t, string(obj["connection"]))
			assert.JSONEq(t, `"postgres"`, string(conn["type"]))
			assert.JSONEq(t, `"db.prod"`, string(conn["host"]))
			assert.JSONEq(t, `5432`, string(conn["port"]))
			assert.NotContains(t, conn, "login")
			if c.password == nil {
				assert.NotContains(t, conn, "password", "the platform withheld it, and so does the json")
			} else {
				assert.JSONEq(t, `"hunter2"`, string(conn["password"]))
			}
			mc.AssertExpectations(t)
		})
	}
}

// The variable link report says what the workspace variable is and where it
// reaches, under snake_case keys: an override is there when the link has one.
func TestEnvVarLinkListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()
	id, dep, other := "clxyz0000000000000000000a", "clxyz0000000000000000000b", "clxyz0000000000000000000c"
	autoLink := true
	links := []astrov1.EnvironmentObjectLink{{
		ScopeEntityId:                dep,
		EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: "eu"},
		SetFields:                    []string{"value"},
	}}
	excludes := []astrov1.EnvironmentObjectExcludeLink{{ScopeEntityId: other}}
	mc := mockEnvList(t, false, astrov1.EnvironmentObject{
		Id: &id, ObjectKey: "REGION", Scope: astrov1.EnvironmentObjectScopeWORKSPACE,
		AutoLinkDeployments: &autoLink, Links: &links, ExcludeLinks: &excludes,
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "us"},
	})
	astroV1Client = mc

	out, err := execEnvCmd("variable", "link", "list", "--variable-key", "REGION", "--workspace-id", "ws-test", "-o", "json")
	require.NoError(t, err)
	var got struct {
		ObjectKey           string `json:"object_key"`
		ObjectID            string `json:"object_id"`
		WorkspaceValue      string `json:"workspace_value"`
		IsSecret            *bool  `json:"is_secret"`
		AutoLinkDeployments bool   `json:"auto_link_deployments"`
		Links               []struct {
			DeploymentID  string  `json:"deployment_id"`
			OverrideValue *string `json:"override_value"`
		} `json:"links"`
		ExcludeLinks []string `json:"exclude_links"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, "REGION", got.ObjectKey)
	assert.Equal(t, id, got.ObjectID)
	assert.Equal(t, "us", got.WorkspaceValue)
	require.NotNil(t, got.IsSecret, "is_secret is always published")
	assert.False(t, *got.IsSecret)
	assert.True(t, got.AutoLinkDeployments)
	require.Len(t, got.Links, 1)
	assert.Equal(t, dep, got.Links[0].DeploymentID)
	require.NotNil(t, got.Links[0].OverrideValue)
	assert.Equal(t, "eu", *got.Links[0].OverrideValue)
	assert.Equal(t, []string{other}, got.ExcludeLinks)
	mc.AssertExpectations(t)
}
