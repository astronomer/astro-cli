package env

import (
	"net/http"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func TestEnv(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestScopeValidate() {
	s.Run("requires one of workspace or deployment", func() {
		s.ErrorIs(Scope{}.Validate(), ErrScopeNotSpecified)
	})
	s.Run("rejects both", func() {
		s.ErrorIs(Scope{WorkspaceID: "ws", DeploymentID: "dep"}.Validate(), ErrScopeAmbiguous)
	})
	s.Run("accepts workspace only", func() {
		s.NoError(Scope{WorkspaceID: "ws"}.Validate())
	})
	s.Run("accepts deployment only", func() {
		s.NoError(Scope{DeploymentID: "dep"}.Validate())
	})
}

func (s *Suite) TestListVars() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	deploymentID := cuid.New()
	objType := astrov1.ENVIRONMENTVARIABLE
	limit := defaultListLimit

	s.Run("workspace scope", func() {
		showSecrets := false
		resolveLinked := true
		workspaceID := cuid.New()
		offset := 0
		params := &astrov1.ListEnvironmentObjectsParams{
			WorkspaceId:   &workspaceID,
			ObjectType:    &objType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolveLinked,
			Limit:         &limit,
			Offset:        &offset,
		}
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, params).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrov1.EnvironmentObject{
					{ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
				},
				TotalCount: 1,
			},
		}, nil).Once()

		objs, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
		s.NoError(err)
		s.Len(objs, 1)
		s.Equal("FOO", objs[0].ObjectKey)
		mc.AssertExpectations(s.T())
	})

	s.Run("deployment scope passes ShowSecrets through", func() {
		showSecrets := true
		resolveLinked := false
		offset := 0
		params := &astrov1.ListEnvironmentObjectsParams{
			DeploymentId:  &deploymentID,
			ObjectType:    &objType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolveLinked,
			Limit:         &limit,
			Offset:        &offset,
		}
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, params).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
		}, nil).Once()

		_, err := ListVars(Scope{DeploymentID: deploymentID}, false, true, mc)
		s.NoError(err)
		mc.AssertExpectations(s.T())
	})

	s.Run("rejects empty scope", func() {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := ListVars(Scope{}, true, false, mc)
		s.ErrorIs(err, ErrScopeNotSpecified)
	})

	s.Run("rejects ambiguous scope", func() {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := ListVars(Scope{WorkspaceID: "ws", DeploymentID: "dep"}, true, false, mc)
		s.ErrorIs(err, ErrScopeAmbiguous)
	})
}

func (s *Suite) TestGetVarByKey() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
		}},
	}, nil).Once()

	got, err := GetVar("FOO", Scope{WorkspaceID: workspaceID}, false, mc)
	s.NoError(err)
	s.Equal("FOO", got.ObjectKey)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestGetVarNotFound() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
	}, nil).Once()

	_, err := GetVar("MISSING", Scope{WorkspaceID: workspaceID}, false, mc)
	s.ErrorIs(err, ErrNotFound)
}

func (s *Suite) TestCreateVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()
	value := "bar"
	isSecret := false

	expectedBody := astrov1.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     "FOO",
		ObjectType:    astrov1.CreateEnvironmentObjectRequestObjectTypeENVIRONMENTVARIABLE,
		Scope:         astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE,
		ScopeEntityId: workspaceID,
		EnvironmentVariable: &astrov1.CreateEnvironmentObjectEnvironmentVariableRequest{
			Value:    &value,
			IsSecret: &isSecret,
		},
	}

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, expectedBody).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()

	got, err := CreateVar(Scope{WorkspaceID: workspaceID}, "FOO", value, isSecret, nil, mc)
	s.NoError(err)
	s.Equal("FOO", got.ObjectKey)
	s.NotNil(got.Id)
	s.Equal(createdID, *got.Id)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestUpdateVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()
	newValue := "newval"

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	// GetVar fall-through (key lookup) -> list
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO"},
		}},
	}, nil).Once()
	emptyLinks := []astrov1.UpdateEnvironmentObjectLinkRequest{}
	emptyExcludes := []astrov1.ExcludeLinkEnvironmentObjectRequest{}
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id, astrov1.UpdateEnvironmentObjectJSONRequestBody{
		EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{Value: &newValue},
		Links:               &emptyLinks,
		ExcludeLinks:        &emptyExcludes,
	}).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	got, err := UpdateVar("FOO", Scope{WorkspaceID: workspaceID}, newValue, nil, mc)
	s.NoError(err)
	s.Equal("FOO", got.ObjectKey)
	mc.AssertExpectations(s.T())
}

// Regression: a value-only update must round-trip the object's existing
// Links/ExcludeLinks. Omitting the arrays makes the platform drop every
// deployment link on the var (and with it any override the deployment had).
func (s *Suite) TestUpdateVarRoundTripsLinks() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()
	depID := cuid.New()
	exDepID := cuid.New()
	newValue := "newval"

	obj := envVarObj(id, "oldval", nil, []astrov1.EnvironmentObjectLink{
		{
			Scope:                        astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
			ScopeEntityId:                depID,
			EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: "prod-override"},
		},
	}, []astrov1.EnvironmentObjectExcludeLink{
		{
			Scope:         astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT,
			ScopeEntityId: exDepID,
		},
	})

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{obj}},
	}, nil).Once()

	expectedLinks := []astrov1.UpdateEnvironmentObjectLinkRequest{
		{
			Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(astrov1.EnvironmentObjectLinkScopeDEPLOYMENT),
			ScopeEntityId: depID,
			Overrides:     newOverrideRequest("prod-override"),
		},
	}
	expectedExcludes := []astrov1.ExcludeLinkEnvironmentObjectRequest{
		{
			Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScope(astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT),
			ScopeEntityId: exDepID,
		},
	}
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id, astrov1.UpdateEnvironmentObjectJSONRequestBody{
		EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{Value: &newValue},
		Links:               &expectedLinks,
		ExcludeLinks:        &expectedExcludes,
	}).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	_, err := UpdateVar("FOO", Scope{WorkspaceID: workspaceID}, newValue, nil, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestDeleteVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO"},
		}},
	}, nil).Once()
	mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrov1.DeleteEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
	}, nil).Once()

	s.NoError(DeleteVar("FOO", Scope{WorkspaceID: workspaceID}, mc))
	mc.AssertExpectations(s.T())
}
