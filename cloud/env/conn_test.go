package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateConnRequiresType() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	_, err := CreateConn(Scope{WorkspaceID: cuid.New()}, "db_main", ConnInput{}, mc)
	s.ErrorContains(err, "connection type is required")
}

func (s *Suite) TestCreateConn() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()
	host := "db.example.com"

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, mock.MatchedBy(func(body astrocore.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "db_main" &&
			body.ObjectType == astrocore.CreateEnvironmentObjectRequestObjectTypeCONNECTION &&
			body.Connection != nil &&
			body.Connection.Type == "postgres" &&
			body.Connection.Host != nil && *body.Connection.Host == host
	})).Return(&astrocore.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()
	mc.On("GetEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, createdID).Return(&astrocore.GetEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &createdID, ObjectKey: "db_main"},
	}, nil).Once()

	got, err := CreateConn(Scope{WorkspaceID: workspaceID}, "db_main", ConnInput{Type: "postgres", Host: &host}, mc)
	s.NoError(err)
	s.Equal("db_main", got.ObjectKey)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestDeleteConn() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	// CUID path: no GetVar lookup, direct DELETE.
	mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrocore.DeleteEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
	}, nil).Once()

	s.NoError(DeleteConn(id, Scope{WorkspaceID: cuid.New()}, mc))
	mc.AssertExpectations(s.T())
}
