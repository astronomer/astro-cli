package teams

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

var errMockHouston = errors.New("mock houston error")

func TestGet(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", Name: "test-name"}, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{{ID: "user-id", Username: "username"}}, nil).Once()

		err := Get("test-id", true, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), "test-name")
		assert.Contains(t, buf.String(), "user-id")
		assert.Contains(t, buf.String(), "username")
		mockClient.AssertExpectations(t)
	})

	t.Run("getTeam error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Get("test-id", true, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("getTeamUsers error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", Name: "test-name"}, nil).Once()
		mockClient.On("GetTeamUsers", "test-id").Return([]houston.User{}, errMockHouston).Once()

		err := Get("test-id", true, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", "", ListTeamLimit).Return(houston.ListTeamsResp{Count: 1, Teams: []houston.Team{{ID: "test-id", Name: "test-name"}}}, nil)

		err := List(mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), "test-name")
		mockClient.AssertExpectations(t)
	})

	t.Run("listTeams error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("ListTeams", "", ListTeamLimit).Return(houston.ListTeamsResp{}, errMockHouston)

		err := List(mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}

func TestUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", "test-id", houston.SystemAdminRole).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(t)
	})

	t.Run("success to set None", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", "test-id", houston.SystemAdminRole).Return(houston.SystemAdminRole, nil).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id")
		assert.Contains(t, buf.String(), houston.SystemAdminRole)
		mockClient.AssertExpectations(t)
	})

	t.Run("no role specified", func(t *testing.T) {
		buf := new(bytes.Buffer)

		err := Update("test-id", "", nil, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), noRoleSpecifiedMsg)
	})

	t.Run("invalid role", func(t *testing.T) {
		buf := new(bytes.Buffer)

		err := Update("test-id", "invalid-role-string", nil, buf)
		assert.Contains(t, err.Error(), "invalid role: invalid-role-string")
	})

	t.Run("CreateTeamSystemRoleBinding error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("CreateTeamSystemRoleBinding", "test-id", houston.SystemAdminRole).Return("", errMockHouston).Once()

		err := Update("test-id", houston.SystemAdminRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("GetTeam error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(nil, errMockHouston).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})

	t.Run("No role set already", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{}}, nil).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Role for the team test-id already set to None, nothing to update")
		mockClient.AssertExpectations(t)
	})

	t.Run("DeleteTeamSystemRoleBinding error", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockClient := new(houston_mocks.ClientInterface)
		mockClient.On("GetTeam", "test-id").Return(&houston.Team{ID: "test-id", RoleBindings: []houston.RoleBinding{{Role: houston.SystemAdminRole}}}, nil).Once()
		mockClient.On("DeleteTeamSystemRoleBinding", "test-id", houston.SystemAdminRole).Return("", errMockHouston).Once()

		err := Update("test-id", houston.NoneTeamRole, mockClient, buf)
		assert.ErrorIs(t, err, errMockHouston)
		mockClient.AssertExpectations(t)
	})
}
