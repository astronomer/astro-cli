package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestAddWorkspaceUser() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			AddWorkspaceUser: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "description",
				Users: []User{
					{
						ID: "id",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Username: "test",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.AddWorkspaceUser(AddWorkspaceUserRequest{"workspace-id", "email", "role"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.AddWorkspaceUser)
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

		_, err := api.AddWorkspaceUser(AddWorkspaceUserRequest{"workspace-id", "email", "role"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteWorkspaceUser() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			RemoveWorkspaceUser: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "description",
				Users: []User{
					{
						ID: "id",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Username: "test",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.DeleteWorkspaceUser(DeleteWorkspaceUserRequest{"workspace-id", "user-id"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.RemoveWorkspaceUser)
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

		_, err := api.DeleteWorkspaceUser(DeleteWorkspaceUserRequest{"workspace-id", "user-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListWorkspaceUserAndRoles() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceGetUsers: []WorkspaceUserRoleBindings{
				{
					ID:       "user-id",
					Username: "test@astronomer.com",
					FullName: "test",
					Emails:   []Email{{Address: "test@astronomer.com"}},
					RoleBindings: []RoleBinding{
						{
							Role: WorkspaceViewerRole,
						},
					},
				},
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

		response, err := api.ListWorkspaceUserAndRoles("workspace-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.WorkspaceGetUsers)
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

		_, err := api.ListWorkspaceUserAndRoles("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListWorkspacePaginatedUserAndRoles() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspacePaginatedGetUsers: []WorkspaceUserRoleBindings{
				{
					ID:       "user-id",
					Username: "test@astronomer.com",
					FullName: "test",
					Emails:   []Email{{Address: "test@astronomer.com"}},
					RoleBindings: []RoleBinding{
						{
							Role: WorkspaceViewerRole,
						},
					},
				},
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

		response, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.WorkspacePaginatedGetUsers)
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

		_, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateWorkspaceUserAndRole() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceUpsertUserRole: WorkspaceAdminRole,
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

		response, err := api.UpdateWorkspaceUserRole(UpdateWorkspaceUserRoleRequest{"workspace-id", "test@test.com", WorkspaceAdminRole})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.WorkspaceUpsertUserRole)
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

		_, err := api.UpdateWorkspaceUserRole(UpdateWorkspaceUserRoleRequest{"workspace-id", "test@test.com", WorkspaceAdminRole})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetWorkspaceUserRole() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceGetUser: WorkspaceUserRoleBindings{
				RoleBindings: []RoleBinding{
					{
						Role: DeploymentAdminRole,
					},
					{
						Role: WorkspaceAdminRole,
					},
				},
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

		response, err := api.GetWorkspaceUserRole(GetWorkspaceUserRoleRequest{"workspace-id", "email"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.WorkspaceGetUser)
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

		_, err := api.GetWorkspaceUserRole(GetWorkspaceUserRoleRequest{"workspace-id", "email"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
