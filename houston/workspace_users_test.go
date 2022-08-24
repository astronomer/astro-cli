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

		response, err := api.AddWorkspaceUser(AddWorkspaceUserRequest{"workspace-id", "email", "role"})
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

		_, err := api.AddWorkspaceUser(AddWorkspaceUserRequest{"workspace-id", "email", "role"})
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
		houstonMethodAvailabilityByVersion["AddWorkspaceUser"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.AddWorkspaceUser(AddWorkspaceUserRequest{"workspace-id", "email", "role"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"AddWorkspaceUser"})
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

		response, err := api.DeleteWorkspaceUser(DeleteWorkspaceUserRequest{"workspace-id", "user-id"})
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

		_, err := api.DeleteWorkspaceUser(DeleteWorkspaceUserRequest{"workspace-id", "user-id"})
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
		houstonMethodAvailabilityByVersion["DeleteWorkspaceUser"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteWorkspaceUser(DeleteWorkspaceUserRequest{"workspace-id", "user-id"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"DeleteWorkspaceUser"})
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
		houstonMethodAvailabilityByVersion["ListWorkspaceUserAndRoles"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListWorkspaceUserAndRoles("workspace-id")
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListWorkspaceUserAndRoles"})
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

		response, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
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

		_, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
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
		houstonMethodAvailabilityByVersion["ListWorkspacePaginatedUserAndRoles"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListWorkspacePaginatedUserAndRoles"})
	})
}

func TestUpdateWorkspaceUserAndRole(t *testing.T) {
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

		response, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
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

		_, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
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
		houstonMethodAvailabilityByVersion["ListWorkspacePaginatedUserAndRoles"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListWorkspacePaginatedUserAndRoles(PaginatedWorkspaceUserRolesRequest{"workspace-id", "cursor-id", 100})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListWorkspacePaginatedUserAndRoles"})
	})
}

func TestGetWorkspaceUserRole(t *testing.T) {
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

		response, err := api.GetWorkspaceUserRole(GetWorkspaceUserRoleRequest{"workspace-id", "email"})
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

		_, err := api.GetWorkspaceUserRole(GetWorkspaceUserRoleRequest{"workspace-id", "email"})
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
		houstonMethodAvailabilityByVersion["GetWorkspaceUserRole"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetWorkspaceUserRole(GetWorkspaceUserRoleRequest{"workspace-id", "email"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"GetWorkspaceUserRole"})
	})
}
