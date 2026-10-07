package astro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
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

// A secret variable's value is null unless --include-secrets asked for it,
// whatever the platform returned, and then the value; a value that is really
// empty is "", so a script tells the two apart. is_secret says which it is
// either way, and set_fields that a hidden secret is set. The same holds for
// an Airflow variable, and for a link's override of either.
func TestEnvVarGetJSONPublishesSecretsOnlyWhenAsked(t *testing.T) {
	for _, c := range []struct {
		name     string
		flags    []string
		secret   bool
		apiValue string
		want     string // the raw json of value
	}{
		{"a secret without --include-secrets", nil, true, "", `null`},
		{"a secret the platform returned anyway", nil, true, "leaked", `null`},
		{"a secret with --include-secrets", []string{"--include-secrets"}, true, "shh", `"shh"`},
		{"an empty secret with --include-secrets", []string{"--include-secrets"}, true, "", `""`},
		{"an empty plain value", nil, false, "", `""`},
	} {
		for _, n := range envNouns[:2] { // variable and airflow-variable
			t.Run(n.noun+"/"+c.name, func(t *testing.T) {
				testUtil.InitTestConfig(testUtil.LocalPlatform)
				defer resetEnvFlags()
				obj := astrov1.EnvironmentObject{
					ObjectKey:     "TOKEN",
					ObjectType:    n.typ,
					Scope:         astrov1.EnvironmentObjectScopeWORKSPACE,
					ScopeEntityId: "ws-test",
					SetFields:     []string{"value"},
				}
				link := astrov1.EnvironmentObjectLink{ScopeEntityId: "dep"}
				if n.typ == astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE {
					obj.EnvironmentVariable = &astrov1.EnvironmentObjectEnvironmentVariable{IsSecret: c.secret, Value: c.apiValue}
					link.EnvironmentVariableOverrides = &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: c.apiValue}
				} else {
					obj.AirflowVariable = &astrov1.EnvironmentObjectAirflowVariable{IsSecret: c.secret, Value: c.apiValue}
					link.AirflowVariableOverrides = &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: c.apiValue}
				}
				obj.Links = &[]astrov1.EnvironmentObjectLink{link}
				mc := mockEnvList(t, len(c.flags) > 0, obj)
				astroV1Client = mc

				out, err := execEnvCmd(append([]string{n.noun, "get", "TOKEN", "--workspace-id", "ws-test", "-o", "json"}, c.flags...)...)
				require.NoError(t, err)
				valueKey, overrideKey := "environment_variable", "environment_variable_overrides"
				if n.typ == astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE {
					valueKey, overrideKey = "airflow_variable", "airflow_variable_overrides"
				}
				var got struct {
					ObjectKey string                       `json:"object_key"`
					SetFields []string                     `json:"set_fields"`
					Links     []map[string]json.RawMessage `json:"links"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &got), out)
				var value, override struct {
					IsSecret bool            `json:"is_secret"`
					Value    json.RawMessage `json:"value"`
				}
				require.NoError(t, json.Unmarshal(decodeObject(t, out)[valueKey], &value), out)
				assert.Equal(t, "TOKEN", got.ObjectKey)
				assert.Equal(t, []string{"value"}, got.SetFields)
				assert.Equal(t, c.secret, value.IsSecret)
				assert.Equal(t, c.want, string(value.Value))
				require.Len(t, got.Links, 1)
				require.NoError(t, json.Unmarshal(got.Links[0][overrideKey], &override), out)
				assert.Equal(t, c.want, string(override.Value), "the link's override")
				mc.AssertExpectations(t)
			})
		}
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

// The link reports hide a secret's value the way the object does: without
// --include-secrets the workspace value and a link's override are null,
// whatever the platform returned, while a link with no override has none;
// with it they are the values.
func TestEnvLinkListJSONHidesASecretsValue(t *testing.T) {
	id, dep, bare := "clxyz0000000000000000000a", "clxyz0000000000000000000b", "clxyz0000000000000000000c"
	for _, c := range []struct {
		name      string
		flags     []string
		value     string // the raw json of each value
		overrides string // what the platform returned
	}{
		{"without --include-secrets", nil, `null`, ""},
		{"when the platform returned the value anyway", nil, `null`, "leaked"},
		{"with --include-secrets", []string{"--include-secrets"}, `"eu"`, "eu"},
	} {
		t.Run("variable/"+c.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			links := []astrov1.EnvironmentObjectLink{
				{ScopeEntityId: dep, EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: c.overrides}},
				{ScopeEntityId: bare},
			}
			mc := mockEnvList(t, len(c.flags) > 0, astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "REGION", Scope: astrov1.EnvironmentObjectScopeWORKSPACE, Links: &links,
				EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: c.overrides, IsSecret: true},
			})
			astroV1Client = mc

			out, err := execEnvCmd(append([]string{envNouns[0].noun, "link", "list", "--variable-key", "REGION", "--workspace-id", "ws-test", "-o", "json"}, c.flags...)...)
			require.NoError(t, err)
			fields := decodeObject(t, out)
			assert.Equal(t, c.value, string(fields["workspace_value"]))
			var got []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(fields["links"], &got), out)
			require.Len(t, got, 2)
			assert.Equal(t, c.value, string(got[0]["override_value"]))
			assert.NotContains(t, got[1], "override_value", "a link with no override has none")
			mc.AssertExpectations(t)
		})
		t.Run("airflow-variable/"+c.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()
			links := []astrov1.EnvironmentObjectLink{{
				ScopeEntityId: dep, SetFields: []string{"value"},
				AirflowVariableOverrides: &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: c.overrides},
			}}
			mc := mockEnvList(t, len(c.flags) > 0, astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "region", Scope: astrov1.EnvironmentObjectScopeWORKSPACE, Links: &links,
				AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: c.overrides, IsSecret: true},
			})
			astroV1Client = mc

			out, err := execEnvCmd(append([]string{"airflow-variable", "link", "list", "--airflow-variable-key", "region", "--workspace-id", "ws-test", "-o", "json"}, c.flags...)...)
			require.NoError(t, err)
			var got struct {
				Links []struct {
					Overrides map[string]json.RawMessage `json:"overrides"`
				} `json:"links"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &got), out)
			require.Len(t, got.Links, 1)
			assert.Equal(t, c.value, string(got.Links[0].Overrides["value"]))
			mc.AssertExpectations(t)
		})
	}
}

// Without --include-secrets a connection's and a metrics export's credentials
// are absent from the json even when the platform's answer carries them: the
// password, the basic token, an extra key the connection's auth type marks
// secret, and a link override's password. Every other field, and an extra
// key that is not secret, is there. With the flag, everything is.
func TestEnvReadsMaskCredentialsTheAPIReturned(t *testing.T) {
	host, pw, tok, id := "db.prod", "s3cret-pw", "s3cret-tok", "clxyz0000000000000000000a"
	conn := func() astrov1.EnvironmentObject {
		extra := map[string]any{"region": "eu", "aws_secret_access_key": "s3cret-key"}
		links := []astrov1.EnvironmentObjectLink{{
			Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: "clxyz0000000000000000000b",
			ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: &host, Password: &pw},
			SetFields:           []string{"host", "password"},
		}}
		return astrov1.EnvironmentObject{
			Id: &id, ObjectKey: "db", ObjectType: astrov1.EnvironmentObjectObjectTypeCONNECTION,
			Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: "ws-test",
			SetFields: []string{"extra.aws_secret_access_key", "extra.region", "host", "password"},
			Links:     &links,
			Connection: &astrov1.EnvironmentObjectConnection{
				Type: "aws", Host: &host, Password: &pw, Extra: &extra,
				ConnectionAuthType: &astrov1.ConnectionAuthType{Parameters: []astrov1.ConnectionAuthTypeParameter{
					{AirflowParamName: "aws_secret_access_key", IsSecret: true, IsInExtra: true},
					{AirflowParamName: "region", IsInExtra: true},
				}},
			},
		}
	}
	metrics := astrov1.EnvironmentObject{
		ObjectKey: "m", ObjectType: astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT,
		Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: "ws-test",
		SetFields:     []string{"basicToken", "endpoint", "exporterType", "password"},
		MetricsExport: &astrov1.EnvironmentObjectMetricsExport{Endpoint: "https://m", ExporterType: "PROMETHEUS", Password: &pw, BasicToken: &tok},
	}
	for _, c := range []struct {
		name string
		obj  astrov1.EnvironmentObject
		args []string
		// secrets are what the platform's answer carries for this view, and
		// plain a field of it that is not secret.
		secrets []string
		plain   string
	}{
		{"connection get", conn(), []string{"connection", "get", "db"}, []string{"s3cret-pw", "s3cret-key"}, `"region":"eu"`},
		{"connection list", conn(), []string{"connection", "list"}, []string{"s3cret-pw", "s3cret-key"}, `"region":"eu"`},
		{"connection link list", conn(), []string{"connection", "link", "list", "--connection-key", "db"}, []string{"s3cret-pw"}, `"host":"db.prod"`},
		{"metrics-export get", metrics, []string{"metrics-export", "get", "m"}, []string{"s3cret-pw", "s3cret-tok"}, `"endpoint":"https://m"`},
		{"metrics-export list", metrics, []string{"metrics-export", "list"}, []string{"s3cret-pw", "s3cret-tok"}, `"endpoint":"https://m"`},
	} {
		for _, include := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/include-secrets=%v", c.name, include), func(t *testing.T) {
				testUtil.InitTestConfig(testUtil.LocalPlatform)
				defer resetEnvFlags()
				mc := mockEnvList(t, include, c.obj)
				astroV1Client = mc
				args := append(slices.Clone(c.args), "--workspace-id", "ws-test", "-o", "json")
				if include {
					args = append(args, "--include-secrets")
				}
				out, err := execEnvCmd(args...)
				require.NoError(t, err)
				check := assert.NotContains
				if include {
					check = assert.Contains
				}
				for _, secret := range c.secrets {
					check(t, out, secret, "published only when asked for, whatever the platform sent")
				}
				assert.Contains(t, out, c.plain, "what is not secret is there either way")
				mc.AssertExpectations(t)
			})
		}
	}
}
