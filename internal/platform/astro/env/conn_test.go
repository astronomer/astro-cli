package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateConnRequiresType() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	_, err := CreateConn(Scope{WorkspaceID: cuid.New()}, "db_main", ConnInput{}, mc)
	s.ErrorContains(err, "connection type is required")
}

func (s *Suite) TestCreateConn() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()
	host := "db.example.com"

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "db_main" &&
			body.ObjectType == astrov1.CreateEnvironmentObjectRequestObjectTypeCONNECTION &&
			body.Connection != nil &&
			body.Connection.Type == "postgres" &&
			body.Connection.Host != nil && *body.Connection.Host == host
	})).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
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

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	// CUID path: no key-to-ID lookup, direct DELETE.
	mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrov1.DeleteEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
	}, nil).Once()

	s.NoError(DeleteConn(id, Scope{WorkspaceID: cuid.New()}, mc))
	mc.AssertExpectations(s.T())
}
