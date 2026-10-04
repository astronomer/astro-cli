package astro

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func linkedConnObj(id string, links *[]astrov1.EnvironmentObjectLink) *astrov1.EnvironmentObject {
	host := "db.internal"
	return &astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     "db",
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.CONNECTION),
		Scope:         astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId: cuid.New(),
		Connection:    &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: &host},
		Links:         links,
	}
}

func mockListReturns(mc *astrov1_mocks.ClientWithResponsesInterface, obj *astrov1.EnvironmentObject) {
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{*obj}},
	}, nil).Once()
}

func TestEnvConnLinkSetSendsTheFieldOverrides(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id, depID := cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mockListReturns(mc, linkedConnObj(id, nil))
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Connection == nil || b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			o := (*b.Links)[0].Overrides
			return o != nil && o.Connection != nil &&
				*o.Connection.Host == "db.prod" && *o.Connection.Port == 6432 &&
				o.Connection.Login == nil
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "db"},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("connection", "link", "set", "--connection-key", "db", "--workspace-id", cuid.New(),
		"--deployment-id", depID, "--host", "db.prod", "--port", "6432")
	assert.NoError(t, err)
	assert.Contains(t, out, "Linked db to deployment "+depID+" (override applied)")
	assert.Contains(t, out, deploymentPickupNote)
	mc.AssertExpectations(t)
}

// An override and --exclude are two different requests; passing both would
// silently drop one.
func TestEnvConnLinkSetRefusesOverrideWithExclude(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("connection", "link", "set", "--connection-key", "db", "--workspace-id", cuid.New(),
		"--deployment-id", cuid.New(), "--host", "db.prod", "--exclude")
	assert.ErrorContains(t, err, "none of the others can be")
	mc.AssertExpectations(t)
}

func TestEnvAirflowVarLinkSetAndList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id, depID := cuid.New(), cuid.New()
	obj := astrov1.EnvironmentObject{
		Id:              &id,
		ObjectKey:       "region",
		ObjectType:      astrov1.EnvironmentObjectObjectType(astrov1.AIRFLOWVARIABLE),
		Scope:           astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:   cuid.New(),
		AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: "us-east-1"},
	}

	t.Run("set", func(t *testing.T) {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mockListReturns(mc, &obj)
		mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id,
			mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
				return b.AirflowVariable != nil && *b.AirflowVariable.Value == "us-east-1" &&
					len(*b.Links) == 1 && *(*b.Links)[0].Overrides.AirflowVariable.Value == "eu-west-1"
			}),
		).Return(&astrov1.UpdateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "region"},
		}, nil).Once()
		astroV1Client = mc

		out, err := execEnvCmd("airflow-var", "link", "set", "--airflow-variable-key", "region", "--workspace-id", cuid.New(),
			"--deployment-id", depID, "--value", "eu-west-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Linked region to deployment "+depID+" (override applied)")
		mc.AssertExpectations(t)
	})

	t.Run("list as json", func(t *testing.T) {
		linked := obj
		links := []astrov1.EnvironmentObjectLink{{
			Scope:                    astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
			ScopeEntityId:            depID,
			AirflowVariableOverrides: &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: "eu-west-1"},
			SetFields:                []string{"value"},
		}}
		linked.Links = &links
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mockListReturns(mc, &linked)
		astroV1Client = mc

		out, err := execEnvCmd("airflow-variable", "link", "list", "--airflow-variable-key", "region", "--workspace-id", cuid.New(), "--format", "json")
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "region", got["objectKey"])
		assert.Equal(t, []any{}, got["excludeLinks"])
		assert.Equal(t, []any{map[string]any{
			"deploymentId": depID,
			"overrides":    map[string]any{"value": "eu-west-1"},
			"setFields":    []any{"value"},
		}}, got["links"])
		mc.AssertExpectations(t)
	})
}

// A password the platform masks still shows as set, rather than as no
// override at all.
func TestEnvConnLinkListShowsAMaskedPasswordAsHidden(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	host := "db.prod"
	links := []astrov1.EnvironmentObjectLink{{
		Scope:               astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:       cuid.New(),
		ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: &host},
		SetFields:           []string{"host", "password"},
	}}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mockListReturns(mc, linkedConnObj(cuid.New(), &links))
	astroV1Client = mc

	out, err := execEnvCmd("conn", "link", "list", "--connection-key", "db", "--workspace-id", cuid.New())
	assert.NoError(t, err)
	assert.Contains(t, out, "host=db.prod password=(hidden, use --include-secrets)")
	mc.AssertExpectations(t)
}
