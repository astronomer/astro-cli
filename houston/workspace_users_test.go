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

func TestAddWorkspaceUser(t *testing.T) {
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

		response, err := api.AddWorkspaceUser("workspace-id", "email", "role")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.AddWorkspaceUser)
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

		_, err := api.AddWorkspaceUser("workspace-id", "email", "role")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteWorkspaceUser(t *testing.T) {
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

		response, err := api.DeleteWorkspaceUser("workspace-id", "user-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.RemoveWorkspaceUser)
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

		_, err := api.DeleteWorkspaceUser("workspace-id", "user-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListWorkspaceUserAndRoles(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceGetUsers: []WorkspaceUserRoleBindings{
				{
					ID:       "user-id",
					Username: "test@astronomer.com",
					FullName: "test",
					Emails:   []Email{{Address: "test@astronomer.com"}},
					RoleBindings: []RoleBindingWorkspace{
						{
							Role: WorkspaceViewerRole,
						},
					},
				},
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

		response, err := api.ListWorkspaceUserAndRoles("workspace-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.WorkspaceGetUsers)
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

		_, err := api.ListWorkspaceUserAndRoles("workspace-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListWorkspacePaginatedUserAndRoles(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspacePaginatedGetUsers: []WorkspaceUserRoleBindings{
				{
					ID:       "user-id",
					Username: "test@astronomer.com",
					FullName: "test",
					Emails:   []Email{{Address: "test@astronomer.com"}},
					RoleBindings: []RoleBindingWorkspace{
						{
							Role: WorkspaceViewerRole,
						},
					},
				},
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

		response, err := api.ListWorkspacePaginatedUserAndRoles("workspace-id", "cursor-id", 100)
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.WorkspacePaginatedGetUsers)
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

		_, err := api.ListWorkspacePaginatedUserAndRoles("workspace-id", "cursor-id", 100)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateWorkspaceUserAndRole(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceUpdateUserRole: DeploymentAdminRole,
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

		response, err := api.UpdateWorkspaceUserRole("workspace-id", "email", DeploymentAdminRole)
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.WorkspaceUpdateUserRole)
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

		_, err := api.UpdateWorkspaceUserRole("workspace-id", "email", "role")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetWorkspaceUserRole(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			WorkspaceGetUser: WorkspaceUserRoleBindings{
				RoleBindings: []RoleBindingWorkspace{
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

		response, err := api.GetWorkspaceUserRole("workspace-id", "email")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.WorkspaceGetUser)
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

		_, err := api.GetWorkspaceUserRole("workspace-id", "email")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
