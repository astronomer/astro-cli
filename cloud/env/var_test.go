package env

import (
	"net/http"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
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
	objType := astrocore.ENVIRONMENTVARIABLE
	limit := defaultListLimit

	s.Run("workspace scope", func() {
		showSecrets := false
		resolveLinked := true
		workspaceID := cuid.New()
		params := &astrocore.ListEnvironmentObjectsParams{
			WorkspaceId:   &workspaceID,
			ObjectType:    &objType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolveLinked,
			Limit:         &limit,
		}
		mc := new(astrocore_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, params).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
				{ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
			}},
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
		params := &astrocore.ListEnvironmentObjectsParams{
			DeploymentId:  &deploymentID,
			ObjectType:    &objType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolveLinked,
			Limit:         &limit,
		}
		mc := new(astrocore_mocks.ClientWithResponsesInterface)
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, params).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
		}, nil).Once()

		_, err := ListVars(Scope{DeploymentID: deploymentID}, false, true, mc)
		s.NoError(err)
		mc.AssertExpectations(s.T())
	})

	s.Run("rejects empty scope", func() {
		mc := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := ListVars(Scope{}, true, false, mc)
		s.ErrorIs(err, ErrScopeNotSpecified)
	})

	s.Run("rejects ambiguous scope", func() {
		mc := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := ListVars(Scope{WorkspaceID: "ws", DeploymentID: "dep"}, true, false, mc)
		s.ErrorIs(err, ErrScopeAmbiguous)
	})
}

func (s *Suite) TestGetVarByKey() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO", EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "bar"}},
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

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: nil},
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

	expectedBody := astrocore.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     "FOO",
		ObjectType:    astrocore.CreateEnvironmentObjectRequestObjectTypeENVIRONMENTVARIABLE,
		Scope:         astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE,
		ScopeEntityId: workspaceID,
		EnvironmentVariable: &astrocore.CreateEnvironmentObjectEnvironmentVariableRequest{
			Value:    &value,
			IsSecret: &isSecret,
		},
	}

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, expectedBody).Return(&astrocore.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()

	got, err := CreateVar(Scope{WorkspaceID: workspaceID}, "FOO", value, isSecret, mc)
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

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	// GetVar fall-through (key lookup) -> list
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO"},
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id, astrocore.UpdateEnvironmentObjectJSONRequestBody{
		EnvironmentVariable: &astrocore.UpdateEnvironmentObjectEnvironmentVariableRequest{Value: &newValue},
	}).Return(&astrocore.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	got, err := UpdateVar("FOO", Scope{WorkspaceID: workspaceID}, newValue, mc)
	s.NoError(err)
	s.Equal("FOO", got.ObjectKey)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestDeleteVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	id := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{Id: &id, ObjectKey: "FOO"},
		}},
	}, nil).Once()
	mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrocore.DeleteEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
	}, nil).Once()

	s.NoError(DeleteVar("FOO", Scope{WorkspaceID: workspaceID}, mc))
	mc.AssertExpectations(s.T())
}
