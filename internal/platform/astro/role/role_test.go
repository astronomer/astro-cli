package role

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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
	role1        = astrov1.Role{
		Name:        "role 1",
		Description: &description,
		Id:          "role1-id",
	}
	role2 = astrov1.Role{
		Name:        "role 2",
		Description: &description,
		Id:          "role2-id",
	}
	roles = []astrov1.Role{
		role1,
		role2,
	}
	defaultRole1 = astrov1.DefaultRole{
		Name:        "default role 1",
		Description: &description,
	}
	defaultRole2 = astrov1.DefaultRole{
		Name:        "default role 2",
		Description: &description,
	}
	defaultRoles = []astrov1.DefaultRole{
		defaultRole1,
		defaultRole2,
	}
	ListRolesResponseOK = astrov1.ListRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.RolesPaginated{
			Limit:        1,
			Offset:       0,
			TotalCount:   1,
			Roles:        roles,
			DefaultRoles: &defaultRoles,
		},
	}
	errorBodyList, _ = json.Marshal(astrov1.Error{
		Message: "failed to list roles",
	})
	ListRolesResponseError = astrov1.ListRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
)

func (s *Suite) TestListData() {
	s.Run("the default roles first, then the Organization's own", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseOK, nil).Twice()
		list, err := ListData(mockClient, true)
		s.NoError(err)
		s.Equal([]RoleInfo{
			{Name: "default role 1", Description: description, IsDefault: true},
			{Name: "default role 2", Description: description, IsDefault: true},
			{Name: "role 1", ID: "role1-id", Description: description},
			{Name: "role 2", ID: "role2-id", Description: description},
			{Name: "role 1", ID: "role1-id", Description: description},
			{Name: "role 2", ID: "role2-id", Description: description},
		}, list.Roles, "the mock answers every page alike, so the second page repeats the first")
	})

	s.Run("no default roles unless asked for", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseOK, nil).Twice()
		list, err := ListData(mockClient, false)
		s.NoError(err)
		for _, r := range list.Roles {
			s.False(r.IsDefault, r.Name)
		}
	})

	s.Run("a role with no description, and an answer with no default roles", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListRolesResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.RolesPaginated{Roles: []astrov1.Role{{Name: "bare", Id: "bare-id", ScopeType: astrov1.RoleScopeTypeWORKSPACE}}},
		}, nil).Once()
		list, err := ListData(mockClient, true)
		s.NoError(err)
		s.Equal([]RoleInfo{{Name: "bare", ID: "bare-id", ScopeType: "WORKSPACE"}}, list.Roles)
	})

	s.Run("error path when ListRolesWithResponse return network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := ListData(mockClient, true)
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListRolesWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListRolesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListRolesResponseError, nil).Twice()
		_, err := ListData(mockClient, true)
		s.EqualError(err, "failed to list roles")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := ListData(mockClient, true)
		s.Error(err)
	})
}
