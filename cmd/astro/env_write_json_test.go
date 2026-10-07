package astro

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

// The `astro env` writes under -o json: a set publishes the object as it now
// is, a delete the object as it was, a link change the links as it left
// them, and `set --from-file` what it did with each key. Text stays the line
// each always printed, which the other env tests check.

const (
	wjWorkspace  = "clwjwsaaaaaaaaaaaaaaaaaaa"
	wjDeployment = "clwjdepaaaaaaaaaaaaaaaaaa"
	wjID         = "clwjobjaaaaaaaaaaaaaaaaaa"
	wjCreatedID  = "clwjnewaaaaaaaaaaaaaaaaaa"
)

// wjObject is the workspace object a key lookup finds: of type typ, linked to
// wjDeployment when the key says LINKED, and excluding it when it says EXCL.
// A key starting NEW is not there.
func wjObject(key string, typ astrov1.EnvironmentObjectObjectType) astrov1.EnvironmentObject {
	id := wjID
	o := astrov1.EnvironmentObject{
		Id: &id, ObjectKey: key, ObjectType: typ,
		Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wjWorkspace,
		SetFields: []string{"value"},
	}
	switch typ {
	case astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE:
		o.EnvironmentVariable = &astrov1.EnvironmentObjectEnvironmentVariable{Value: "stored"}
	case astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE:
		o.AirflowVariable = &astrov1.EnvironmentObjectAirflowVariable{Value: "stored"}
	case astrov1.EnvironmentObjectObjectTypeCONNECTION:
		h := "db.internal"
		o.Connection = &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: &h}
	case astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT:
		o.MetricsExport = &astrov1.EnvironmentObjectMetricsExport{Endpoint: "https://m", ExporterType: "PROMETHEUS"}
	}
	if strings.Contains(key, "LINKED") {
		o.Links = &[]astrov1.EnvironmentObjectLink{{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: wjDeployment}}
	}
	if strings.Contains(key, "EXCL") {
		o.ExcludeLinks = &[]astrov1.EnvironmentObjectExcludeLink{{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: wjDeployment}}
	}
	return o
}

// wjClient answers like the platform for one workspace: a lookup finds
// wjObject, an update answers with the object as the body leaves its links,
// a create answers with wjCreatedID alone, and a GET of that id is the new
// object as the platform holds it, a secret's value masked. Nothing that
// writes is required, so a test asserts what it expects to have been called.
func wjClient(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	ok := &http.Response{StatusCode: http.StatusOK}
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ string, p *astrov1.ListEnvironmentObjectsParams, _ ...astrov1.RequestEditorFn) (*astrov1.ListEnvironmentObjectsResponse, error) {
			typ := astrov1.EnvironmentObjectObjectType(*p.ObjectType)
			objs := []astrov1.EnvironmentObject{}
			switch {
			case p.ObjectKey == nil:
				objs = append(objs, wjObject("A", typ), wjObject("B", typ))
			case !strings.HasPrefix(*p.ObjectKey, "NEW"):
				objs = append(objs, wjObject(*p.ObjectKey, typ))
			}
			return &astrov1.ListEnvironmentObjectsResponse{
				HTTPResponse: ok,
				JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: objs, TotalCount: len(objs)},
			}, nil
		}).Maybe()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything).Return(
		func(_ context.Context, _, _ string, b astrov1.UpdateEnvironmentObjectJSONRequestBody, _ ...astrov1.RequestEditorFn) (*astrov1.UpdateEnvironmentObjectResponse, error) {
			return &astrov1.UpdateEnvironmentObjectResponse{HTTPResponse: ok, JSON200: wjUpdated(&b)}, nil
		}).Maybe()
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: ok, JSON200: &astrov1.CreateEnvironmentObject{Id: wjCreatedID},
	}, nil).Maybe()
	mc.On("GetEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjCreatedID).Return(
		func(_ context.Context, _, _ string, _ ...astrov1.RequestEditorFn) (*astrov1.GetEnvironmentObjectResponse, error) {
			// The type the create asked for, which the test cannot see from
			// here, is read off the last create call.
			typ := astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE
			for i := range mc.Calls {
				if c := &mc.Calls[i]; c.Method == "CreateEnvironmentObjectWithResponse" {
					typ = astrov1.EnvironmentObjectObjectType(c.Arguments.Get(2).(astrov1.CreateEnvironmentObjectJSONRequestBody).ObjectType)
				}
			}
			o := wjObject("NEWK", typ)
			id := wjCreatedID
			o.Id = &id
			if o.EnvironmentVariable != nil {
				o.EnvironmentVariable = &astrov1.EnvironmentObjectEnvironmentVariable{IsSecret: true}
			}
			return &astrov1.GetEnvironmentObjectResponse{HTTPResponse: ok, JSON200: &o}, nil
		}).Maybe()
	// The read back after an exclude: the object the last lookup found, now
	// excluding wjDeployment.
	mc.On("GetEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID).Return(
		func(_ context.Context, _, _ string, _ ...astrov1.RequestEditorFn) (*astrov1.GetEnvironmentObjectResponse, error) {
			var o astrov1.EnvironmentObject
			for i := range mc.Calls {
				if c := &mc.Calls[i]; c.Method == "ListEnvironmentObjectsWithResponse" {
					p := c.Arguments.Get(2).(*astrov1.ListEnvironmentObjectsParams)
					o = wjObject(*p.ObjectKey, astrov1.EnvironmentObjectObjectType(*p.ObjectType))
				}
			}
			o.ExcludeLinks = &[]astrov1.EnvironmentObjectExcludeLink{{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: wjDeployment}}
			return &astrov1.GetEnvironmentObjectResponse{HTTPResponse: ok, JSON200: &o}, nil
		}).Maybe()
	mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID).Return(&astrov1.DeleteEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil).Maybe()
	mc.On("ExcludeLinkingEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything).Return(&astrov1.ExcludeLinkingEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil).Maybe()
	return mc
}

// wjUpdated is the object an update answers with: of the kind the body
// carries a value for, with the links and excludes the body gives it.
func wjUpdated(b *astrov1.UpdateEnvironmentObjectJSONRequestBody) *astrov1.EnvironmentObject {
	typ := astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE
	switch {
	case b.Connection != nil:
		typ = astrov1.EnvironmentObjectObjectTypeCONNECTION
	case b.AirflowVariable != nil:
		typ = astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE
	case b.MetricsExport != nil:
		typ = astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT
	}
	o := wjObject("K", typ)
	if b.Links != nil {
		links := []astrov1.EnvironmentObjectLink{}
		for _, l := range *b.Links {
			link := astrov1.EnvironmentObjectLink{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: l.ScopeEntityId}
			if ov := l.Overrides; ov != nil && ov.EnvironmentVariable != nil && ov.EnvironmentVariable.Value != nil {
				link.EnvironmentVariableOverrides = &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: *ov.EnvironmentVariable.Value}
			}
			if ov := l.Overrides; ov != nil && ov.Connection != nil {
				link.ConnectionOverrides = &astrov1.EnvironmentObjectConnectionOverrides{Host: ov.Connection.Host}
			}
			links = append(links, link)
		}
		o.Links = &links
	}
	if b.ExcludeLinks != nil {
		excludes := []astrov1.EnvironmentObjectExcludeLink{}
		for _, e := range *b.ExcludeLinks {
			excludes = append(excludes, astrov1.EnvironmentObjectExcludeLink{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: e.ScopeEntityId})
		}
		o.ExcludeLinks = &excludes
	}
	return &o
}

// execEnvJSON runs `astro env <args>` through cliout.Execute, the way the CLI
// does, with the flags a previous run set put back first.
func execEnvJSON(t *testing.T, client astrov1.APIClient, args ...string) tokenRun {
	t.Helper()
	resetEnvFlags()
	t.Cleanup(resetEnvFlags)
	return execAstroCmd(t, client, "", newEnvRootCmd, append([]string{"env"}, args...)...)
}

// envNouns are the four object kinds, with the flags a set needs.
var envNouns = []struct {
	noun string
	typ  astrov1.EnvironmentObjectObjectType
	set  []string
}{
	{"variable", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, []string{"--value", "s3cret-value", "--secret"}},
	{"airflow-variable", astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE, []string{"--value", "s3cret-value", "--secret"}},
	{"connection", astrov1.EnvironmentObjectObjectTypeCONNECTION, []string{"--type", "postgres", "--password", "s3cret-value"}},
	{"metrics-export", astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT, []string{"--endpoint", "https://m", "--exporter-type", "PROMETHEUS", "--basic-token", "s3cret-value"}},
}

// A set publishes the object as it now is: an update the object the platform
// answered with, and a create the object read back by the id the create
// returned, so a secret it was given is not echoed onto stdout.
func TestEnvSetJSONPublishesTheObject(t *testing.T) {
	ws := "--workspace-id=" + wjWorkspace
	for _, n := range envNouns {
		t.Run(n.noun+"/update", func(t *testing.T) {
			mc := wjClient(t)
			r := execEnvJSON(t, mc, append([]string{n.noun, "set", "K", ws, "-o", "json"}, n.set...)...)
			require.NoError(t, r.err, r.stdout)
			var got env.ObjectInfo
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, "K", got.ObjectKey)
			assert.Equal(t, string(n.typ), got.ObjectType)
			mc.AssertCalled(t, "UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything)
		})
		t.Run(n.noun+"/create", func(t *testing.T) {
			mc := wjClient(t)
			r := execEnvJSON(t, mc, append([]string{n.noun, "set", "NEWK", ws, "-o", "json"}, n.set...)...)
			require.NoError(t, r.err, r.stdout)
			var got env.ObjectInfo
			decodeOne(t, r.stdout, &got)
			require.NotNil(t, got.ID)
			assert.Equal(t, wjCreatedID, *got.ID)
			assert.Equal(t, "NEWK", got.ObjectKey)
			assert.Equal(t, string(n.typ), got.ObjectType)
			assert.Equal(t, []string{"value"}, got.SetFields, "what the platform holds, not what the create echoed")
			assert.NotContains(t, r.stdout, "s3cret-value")
			mc.AssertCalled(t, "GetEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjCreatedID)
		})
	}
}

// Text has only the key and the id to print, which the create returned, so it
// reads nothing back.
func TestEnvSetTextReadsNothingBack(t *testing.T) {
	for _, n := range envNouns {
		t.Run(n.noun, func(t *testing.T) {
			mc := wjClient(t)
			r := execEnvJSON(t, mc, append([]string{n.noun, "set", "NEWK", "--workspace-id=" + wjWorkspace}, n.set...)...)
			require.NoError(t, r.err)
			assert.Equal(t, "Created NEWK (id: "+wjCreatedID+")\n", r.stdout)
			mc.AssertNotCalled(t, "GetEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func writeDotenv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// set --from-file publishes one outcome per key, in key order: what it
// created, what it updated, and what it left alone for an empty value.
func TestEnvSetFromFileJSON(t *testing.T) {
	for _, noun := range []string{"variable", "airflow-variable"} {
		t.Run(noun, func(t *testing.T) {
			path := writeDotenv(t, "UPD=1\nNEWK=2\nEMPTY=\n")
			r := execEnvJSON(t, wjClient(t), noun, "set", "--from-file", path, "--workspace-id="+wjWorkspace, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			var got env.SetFromFileResult
			decodeOne(t, r.stdout, &got)
			require.Len(t, got.Outcomes, 3)
			assert.Equal(t, env.SetOutcome{Key: "EMPTY", Kind: env.SetSkippedEmpty}, got.Outcomes[0])
			assert.Equal(t, "NEWK", got.Outcomes[1].Key)
			assert.Equal(t, env.SetCreated, got.Outcomes[1].Kind)
			require.NotNil(t, got.Outcomes[1].Object)
			assert.Equal(t, wjCreatedID, *got.Outcomes[1].Object.ID)
			assert.Equal(t, "UPD", got.Outcomes[2].Key)
			assert.Equal(t, env.SetUpdated, got.Outcomes[2].Kind)
			require.NotNil(t, got.Outcomes[2].Object)
		})
	}
	t.Run("a file with no keys", func(t *testing.T) {
		path := writeDotenv(t, "# nothing\n")
		r := execEnvJSON(t, wjClient(t), "variable", "set", "--from-file", path, "--workspace-id="+wjWorkspace, "-o", "json")
		require.NoError(t, r.err)
		fields := decodeOne(t, r.stdout, &env.SetFromFileResult{})
		assert.JSONEq(t, `[]`, string(fields["outcomes"]))
	})
}

// A key that fails stops the import. Text has always printed a line for each
// key set before it; json publishes the one error, naming the key.
func TestEnvSetFromFileFailurePartWay(t *testing.T) {
	failOn := func() *astrov1_mocks.ClientWithResponsesInterface {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(
			func(_ context.Context, _ string, p *astrov1.ListEnvironmentObjectsParams, _ ...astrov1.RequestEditorFn) (*astrov1.ListEnvironmentObjectsResponse, error) {
				o := wjObject(*p.ObjectKey, astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE)
				return &astrov1.ListEnvironmentObjectsResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{o}, TotalCount: 1},
				}, nil
			})
		calls := 0
		mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything).Return(
			func(_ context.Context, _, _ string, _ astrov1.UpdateEnvironmentObjectJSONRequestBody, _ ...astrov1.RequestEditorFn) (*astrov1.UpdateEnvironmentObjectResponse, error) {
				calls++
				if calls == 2 {
					return &astrov1.UpdateEnvironmentObjectResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}, Body: []byte(`{"message":"boom"}`)}, nil
				}
				o := wjObject("A", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE)
				return &astrov1.UpdateEnvironmentObjectResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &o}, nil
			})
		return mc
	}
	path := writeDotenv(t, "A=1\nB=2\nC=3\n")

	r := execEnvJSON(t, failOn(), "variable", "set", "--from-file", path, "--workspace-id="+wjWorkspace)
	require.Error(t, r.err)
	assert.Equal(t, "Updated A\n", r.stdout)
	assert.Contains(t, r.err.Error(), "set B")

	r = execEnvJSON(t, failOn(), "variable", "set", "--from-file", path, "--workspace-id="+wjWorkspace, "-o", "json")
	require.Error(t, r.err)
	assert.Equal(t, cliout.ExitFailure, r.code)
	var got env.SetFromFileResult
	decodeOne(t, r.stdout, &got)
	require.Len(t, got.Outcomes, 2, "A, then B where it stopped; C was not tried")
	assert.Equal(t, "A", got.Outcomes[0].Key)
	assert.Equal(t, env.SetUpdated, got.Outcomes[0].Kind)
	assert.NotNil(t, got.Outcomes[0].Object)
	assert.Equal(t, "B", got.Outcomes[1].Key)
	assert.Equal(t, env.SetFailed, got.Outcomes[1].Kind)
	assert.Nil(t, got.Outcomes[1].Object)
	assert.Contains(t, got.Outcomes[1].Error, "set B")
	assert.Contains(t, r.stderr, "Error: set B", "the error's words go to stderr")
}

// A create whose read back fails has still created the object: the set
// succeeds, publishing the object the create built with its secrets masked,
// and says on stderr that it could not read it back. The pickup note still
// follows.
//
// The object it publishes then is built from what the create was given: its
// set_fields say what was set, secrets included, while the secrets
// themselves, a connection's extra among them, are not there.
func TestEnvSetJSONWhenTheReadBackFails(t *testing.T) {
	wantSet := map[string][]string{
		"variable":         {"value"},
		"airflow-variable": {"value"},
		"connection":       {"extra.aws_secret_access_key", "password", "type"},
		"metrics-export":   {"basicToken", "endpoint", "exporterType"},
	}
	for _, n := range envNouns {
		t.Run(n.noun, func(t *testing.T) {
			set := n.set
			if n.noun == "connection" {
				set = append(slices.Clone(set), "--extra", `{"aws_secret_access_key":"s3cret-value"}`)
			}
			mc := wjClient(t)
			for _, c := range mc.ExpectedCalls {
				if c.Method == "GetEnvironmentObjectWithResponse" {
					c.ReturnArguments = mock.Arguments{&astrov1.GetEnvironmentObjectResponse{
						HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}, Body: []byte(`{"message":"boom"}`),
					}, nil}
				}
			}
			args := append([]string{n.noun, "set", "NEWK", "--workspace-id=" + wjWorkspace, "--auto-link", "-o", "json"}, set...)
			r := execEnvJSON(t, mc, args...)
			require.NoError(t, r.err, r.stderr)
			assert.Equal(t, 0, r.code)
			var got env.ObjectInfo
			fields := decodeOne(t, r.stdout, &got)
			assert.Equal(t, wantSet[n.noun], got.SetFields)
			assert.NotEqual(t, "null", string(fields["set_fields"]))
			if n.noun == "connection" {
				require.NotNil(t, got.Connection)
				assert.Nil(t, got.Connection.Password)
				require.NotNil(t, got.Connection.Extra)
				assert.Equal(t, map[string]any{"aws_secret_access_key": ""}, *got.Connection.Extra,
					"no auth type says which extra keys are secret, so every value is blanked")
			}
			require.NotNil(t, got.ID)
			assert.Equal(t, wjCreatedID, *got.ID)
			assert.Equal(t, "NEWK", got.ObjectKey)
			assert.NotContains(t, r.stdout, "s3cret-value", "the create's echo, masked")
			assert.Contains(t, r.stderr, "reading it back failed")
			if n.noun == "variable" {
				assert.Contains(t, r.stderr, deploymentPickupNote)
			}
		})
	}
}

// Every json path masks what it publishes, whatever the platform answered
// with: here an update, a link set and a delete whose answers carry a
// secret variable's value and its link's override.
func TestEnvWritesMaskSecretsTheAPIReturned(t *testing.T) {
	leaky := func() *astrov1_mocks.ClientWithResponsesInterface {
		id := wjID
		obj := astrov1.EnvironmentObject{
			Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wjWorkspace,
			EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "s3cret-value", IsSecret: true},
			Links: &[]astrov1.EnvironmentObjectLink{{
				Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: wjDeployment,
				EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: "s3cret-override"},
			}},
		}
		ok := &http.Response{StatusCode: http.StatusOK}
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: ok, JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{obj}, TotalCount: 1},
		}, nil)
		mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything).Return(&astrov1.UpdateEnvironmentObjectResponse{
			HTTPResponse: ok, JSON200: &obj,
		}, nil)
		mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID).Return(&astrov1.DeleteEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
		}, nil)
		return mc
	}
	ws, dep := "--workspace-id="+wjWorkspace, "--deployment-id="+wjDeployment
	for _, args := range [][]string{
		{"variable", "set", "K", "--value", "new", ws},
		{"variable", "delete", "K", "--yes", ws},
		{"variable", "link", "set", "--variable-key", "K", dep, "--value", "new", ws},
		{"variable", "link", "delete", "--variable-key", "K", dep, ws},
	} {
		r := execEnvJSON(t, leaky(), append(args, "-o", "json")...)
		require.NoError(t, r.err, args)
		assert.NotContains(t, r.stdout, "s3cret", args)
	}
}

// A delete publishes the object as it was.
func TestEnvDeleteJSONPublishesWhatItDeleted(t *testing.T) {
	for _, n := range envNouns {
		t.Run(n.noun, func(t *testing.T) {
			mc := wjClient(t)
			r := execEnvJSON(t, mc, n.noun, "delete", "K", "--yes", "--workspace-id="+wjWorkspace, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			var got env.ObjectInfo
			decodeOne(t, r.stdout, &got)
			require.NotNil(t, got.ID)
			assert.Equal(t, wjID, *got.ID)
			assert.Equal(t, "K", got.ObjectKey)
			assert.Equal(t, string(n.typ), got.ObjectType)
			mc.AssertCalled(t, "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID)
		})
	}
}

// Under -o json a delete without --yes asks nothing and deletes nothing: it
// fails as input_required, naming --yes.
func TestEnvDeleteJSONNeverAsks(t *testing.T) {
	for _, n := range envNouns {
		t.Run(n.noun, func(t *testing.T) {
			mc := wjClient(t)
			r := execEnvJSON(t, mc, n.noun, "delete", "K", "--workspace-id="+wjWorkspace, "-o", "json")
			require.Error(t, r.err)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, "--yes")
			mc.AssertNotCalled(t, "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// A link change publishes the links as it left them, the report `link list`
// prints: a link set adds the deployment, a link delete takes it away, and
// --exclude does the same to the excludes.
func TestEnvLinkChangesJSON(t *testing.T) {
	ws := "--workspace-id=" + wjWorkspace
	dep := "--deployment-id=" + wjDeployment

	t.Run("variable", func(t *testing.T) {
		var got env.VarLinksReport
		r := execEnvJSON(t, wjClient(t), "variable", "link", "set", "--variable-key", "K", dep, "--value", "own", ws, "-o", "json")
		require.NoError(t, r.err, r.stdout)
		decodeOne(t, r.stdout, &got)
		require.Len(t, got.Links, 1)
		assert.Equal(t, wjDeployment, got.Links[0].DeploymentID)
		require.NotNil(t, got.Links[0].OverrideValue)
		assert.Equal(t, "own", *got.Links[0].OverrideValue)

		got = env.VarLinksReport{}
		r = execEnvJSON(t, wjClient(t), "variable", "link", "delete", "--variable-key", "LINKED", dep, ws, "-o", "json")
		require.NoError(t, r.err, r.stdout)
		decodeOne(t, r.stdout, &got)
		assert.Empty(t, got.Links)

		got = env.VarLinksReport{}
		mc := wjClient(t)
		r = execEnvJSON(t, mc, "variable", "link", "set", "--variable-key", "K", dep, "--exclude", ws, "-o", "json")
		require.NoError(t, r.err, r.stdout)
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, []string{wjDeployment}, got.ExcludeLinks)
		mc.AssertCalled(t, "ExcludeLinkingEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything)

		got = env.VarLinksReport{}
		r = execEnvJSON(t, wjClient(t), "variable", "link", "delete", "--variable-key", "EXCL", dep, "--exclude", ws, "-o", "json")
		require.NoError(t, r.err, r.stdout)
		decodeOne(t, r.stdout, &got)
		assert.Empty(t, got.ExcludeLinks)
	})

	for _, n := range []struct{ noun, keyFlag string }{{"connection", "--connection-key"}, {"airflow-variable", "--airflow-variable-key"}} {
		t.Run(n.noun, func(t *testing.T) {
			var got env.LinksReport
			r := execEnvJSON(t, wjClient(t), n.noun, "link", "set", n.keyFlag, "K", dep, ws, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			decodeOne(t, r.stdout, &got)
			require.Len(t, got.Links, 1)
			assert.Equal(t, wjDeployment, got.Links[0].DeploymentID)

			got = env.LinksReport{}
			r = execEnvJSON(t, wjClient(t), n.noun, "link", "delete", n.keyFlag, "LINKED", dep, ws, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			decodeOne(t, r.stdout, &got)
			assert.Empty(t, got.Links)

			got = env.LinksReport{}
			r = execEnvJSON(t, wjClient(t), n.noun, "link", "set", n.keyFlag, "K", dep, "--exclude", ws, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, []string{wjDeployment}, got.ExcludeLinks)

			got = env.LinksReport{}
			r = execEnvJSON(t, wjClient(t), n.noun, "link", "delete", n.keyFlag, "EXCL", dep, "--exclude", ws, "-o", "json")
			require.NoError(t, r.err, r.stdout)
			decodeOne(t, r.stdout, &got)
			assert.Empty(t, got.ExcludeLinks)
		})
	}

	t.Run("a connection override", func(t *testing.T) {
		var got env.LinksReport
		r := execEnvJSON(t, wjClient(t), "connection", "link", "set", "--connection-key", "K", dep, "--host", "db.prod", ws, "-o", "json")
		require.NoError(t, r.err, r.stdout)
		decodeOne(t, r.stdout, &got)
		require.Len(t, got.Links, 1)
		assert.Equal(t, map[string]any{"host": "db.prod"}, got.Links[0].Overrides)
	})
}

// variable export's text is the dotenv file; its json is the list's.
func TestEnvVarExportJSON(t *testing.T) {
	r := execEnvJSON(t, wjClient(t), "variable", "export", "--workspace-id="+wjWorkspace, "-o", "json")
	require.NoError(t, r.err, r.stdout)
	var got env.VariableList
	decodeOne(t, r.stdout, &got)
	require.Len(t, got.Variables, 2)
	assert.Equal(t, "A", got.Variables[0].ObjectKey)

	r = execEnvJSON(t, wjClient(t), "variable", "export", "--workspace-id="+wjWorkspace)
	require.NoError(t, r.err)
	assert.Equal(t, "A=stored\nB=stored\n", r.stdout)

	r = execEnvJSON(t, wjClient(t), "variable", "export", "--workspace-id="+wjWorkspace, "-o", "dotenv")
	require.NoError(t, r.err)
	assert.Equal(t, "A=stored\nB=stored\n", r.stdout, "dotenv by name is export's text, as list and get take it")
}

// A bad -o on a write is a usage error, before anything is read or written.
func TestEnvWriteRefusesAnUnknownFormat(t *testing.T) {
	ws := "--workspace-id=" + wjWorkspace
	for _, args := range [][]string{
		{"variable", "set", "K", "--value", "x", ws, "-o", "yaml"},
		{"variable", "set", "--from-file", "x.env", ws, "-o", "yaml"},
		{"connection", "delete", "K", "--yes", ws, "-o", "yaml"},
		{"variable", "link", "set", "--variable-key", "K", "--deployment-id=" + wjDeployment, ws, "-o", "yaml"},
		{"airflow-variable", "link", "delete", "--airflow-variable-key", "K", "--deployment-id=" + wjDeployment, ws, "-o", "yaml"},
	} {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		r := execEnvJSON(t, mc, args...)
		require.Error(t, r.err, args)
		assert.Equal(t, cliout.ExitUsage, r.code, args)
		mc.AssertExpectations(t)
	}
}
