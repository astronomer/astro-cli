package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestAddWorkspaceTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			AddWorkspaceTeam: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "description",
				CreatedAt:   "2020-06-25T22:10:42.385Z",
				UpdatedAt:   "2020-06-25T22:10:42.385Z",
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.AddWorkspaceTeam(AddWorkspaceTeamRequest{"workspace-id", "team-id", "role"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.AddWorkspaceTeam)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.AddWorkspaceTeam(AddWorkspaceTeamRequest{"workspace-id", "team-id", "role"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonAPIAvailabilityByVersion["AddWorkspaceTeam"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.AddWorkspaceTeam(AddWorkspaceTeamRequest{"workspace-id", "team-id", "role"})
		assert.ErrorIs(t, err, ErrAPINotImplemented{"AddWorkspaceTeam"})
	})
}

func TestDeleteWorkspaceTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			RemoveWorkspaceTeam: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "description",
				CreatedAt:   "2020-06-25T22:10:42.385Z",
				UpdatedAt:   "2020-06-25T22:10:42.385Z",
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.DeleteWorkspaceTeam(DeleteWorkspaceTeamRequest{"workspace-id", "user-id"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.RemoveWorkspaceTeam)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.DeleteWorkspaceTeam(DeleteWorkspaceTeamRequest{"workspace-id", "user-id"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonAPIAvailabilityByVersion["DeleteWorkspaceTeam"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteWorkspaceTeam(DeleteWorkspaceTeamRequest{"workspace-id", "user-id"})
		assert.ErrorIs(t, err, ErrAPINotImplemented{"DeleteWorkspaceTeam"})
	})
}

func TestListWorkspaceTeamsAndRoles(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := []Team{
		{
			ID: "test-id",
		},
	}
	jsonResponse, err := json.Marshal(Response{Data: ResponseData{
		WorkspaceGetTeams: []Team{
			{
				ID: "test-id",
			},
		},
	}})
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.ListWorkspaceTeamsAndRoles("workspace-id")
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, response)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.ListWorkspaceTeamsAndRoles("workspace-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonAPIAvailabilityByVersion["ListWorkspaceTeamsAndRoles"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListWorkspaceTeamsAndRoles("workspace-id")
		assert.ErrorIs(t, err, ErrAPINotImplemented{"ListWorkspaceTeamsAndRoles"})
	})
}

func TestUpdateWorkspaceTeamAndRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceUpdateTeamRole: WorkspaceAdminRole,
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.UpdateWorkspaceTeamRole(UpdateWorkspaceTeamRoleRequest{"workspace-id", "team-id", WorkspaceAdminRole})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.WorkspaceUpdateTeamRole)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.UpdateWorkspaceTeamRole(UpdateWorkspaceTeamRoleRequest{"workspace-id", "team-id", "role"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonAPIAvailabilityByVersion["UpdateWorkspaceTeamRole"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.UpdateWorkspaceTeamRole(UpdateWorkspaceTeamRoleRequest{"workspace-id", "team-id", "role"})
		assert.ErrorIs(t, err, ErrAPINotImplemented{"UpdateWorkspaceTeamRole"})
	})
}

func TestGetWorkspaceTeamRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	var mockResponse *Team
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.GetWorkspaceTeamRole(GetWorkspaceTeamRoleRequest{"workspace-id", "team-id"})
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, response)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetWorkspaceTeamRole(GetWorkspaceTeamRoleRequest{"workspace-id", "team-id"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonAPIAvailabilityByVersion["GetWorkspaceTeamRole"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetWorkspaceTeamRole(GetWorkspaceTeamRoleRequest{"workspace-id", "team-id"})
		assert.ErrorIs(t, err, ErrAPINotImplemented{"GetWorkspaceTeamRole"})
	})
}
