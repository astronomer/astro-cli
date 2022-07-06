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

		response, err := api.AddWorkspaceTeam("workspace-id", "team-id", "role")
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

		_, err := api.AddWorkspaceTeam("workspace-id", "team-id", "role")
		assert.Contains(t, err.Error(), "Internal Server Error")
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

		response, err := api.DeleteWorkspaceTeam("workspace-id", "user-id")
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

		_, err := api.DeleteWorkspaceTeam("workspace-id", "user-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
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

		response, err := api.UpdateWorkspaceTeamRole("workspace-id", "team-id", WorkspaceAdminRole)
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

		_, err := api.UpdateWorkspaceTeamRole("workspace-id", "team-id", "role")
		assert.Contains(t, err.Error(), "Internal Server Error")
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

		response, err := api.GetWorkspaceTeamRole("workspace-id", "team-id")
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

		_, err := api.GetWorkspaceTeamRole("workspace-id", "team-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
