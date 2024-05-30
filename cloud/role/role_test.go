package role

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestRole(t *testing.T) {
	suite.Run(t, new(Suite))
}

var (
	errorNetwork = errors.New("network error")
	description  = "mockDescription"
	role1        = astrocore.Role{
		Name:        "role 1",
		Description: &description,
		Id:          "role1-id",
	}
	role2 = astrocore.Role{
		Name:        "role 2",
		Description: &description,
		Id:          "role2-id",
	}
	roles = []astrocore.Role{
		role1,
		role2,
	}
	defaultRole1 = astrocore.DefaultRole{
		Name:        "default role 1",
		Description: &description,
	}
	defaultRole2 = astrocore.DefaultRole{
		Name:        "default role 2",
		Description: &description,
	}
	defaultRoles = []astrocore.DefaultRole{
		defaultRole1,
		defaultRole2,
	}
	ListRolesResponseOK = astrocore.ListRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.RolesPaginated{
			Limit:        1,
			Offset:       0,
			TotalCount:   1,
			Roles:        roles,
			DefaultRoles: &defaultRoles,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list roles",
	})
	ListRolesResponseError = astrocore.ListRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
)

func (s *Suite) TestListOrgRole() {
	s.Run("happy path TestListOrgRole", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseOK, nil).Twice()
		err := ListOrgRoles(out, mockClient, true)
		s.NoError(err)
	})

	s.Run("happy path TestListOrgRole - should include default roles false", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseOK, nil).Twice()
		err := ListOrgRoles(out, mockClient, false)
		s.NoError(err)
	})

	s.Run("error path when ListRolesWithResponse return network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgRoles(out, mockClient, true)
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListRolesWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseError, nil).Twice()
		err := ListOrgRoles(out, mockClient, true)
		s.EqualError(err, "failed to list roles")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListOrgRoles(out, mockClient, true)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}
