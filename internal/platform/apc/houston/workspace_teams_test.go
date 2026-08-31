package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestAddWorkspaceTeam() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.AddWorkspaceTeam(AddWorkspaceTeamRequest{"workspace-id", "team-id", "role"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.AddWorkspaceTeam)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.AddWorkspaceTeam(AddWorkspaceTeamRequest{"workspace-id", "team-id", "role"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteWorkspaceTeam() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.DeleteWorkspaceTeam(DeleteWorkspaceTeamRequest{"workspace-id", "user-id"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.RemoveWorkspaceTeam)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.DeleteWorkspaceTeam(DeleteWorkspaceTeamRequest{"workspace-id", "user-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListWorkspaceTeamsAndRoles() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.ListWorkspaceTeamsAndRoles("workspace-id")
		s.NoError(err)
		s.Equal(mockResponse, response)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.ListWorkspaceTeamsAndRoles("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateWorkspaceTeamAndRole() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceUpdateTeamRole: WorkspaceAdminRole,
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.UpdateWorkspaceTeamRole(UpdateWorkspaceTeamRoleRequest{"workspace-id", "team-id", WorkspaceAdminRole})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.WorkspaceUpdateTeamRole)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.UpdateWorkspaceTeamRole(UpdateWorkspaceTeamRoleRequest{"workspace-id", "team-id", "role"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetWorkspaceTeamRole() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	var mockResponse *Team
	jsonResponse, err := json.Marshal(mockResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		response, err := api.GetWorkspaceTeamRole(GetWorkspaceTeamRoleRequest{"workspace-id", "team-id"})
		s.NoError(err)
		s.Equal(mockResponse, response)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetWorkspaceTeamRole(GetWorkspaceTeamRoleRequest{"workspace-id", "team-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
