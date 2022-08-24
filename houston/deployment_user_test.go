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

func TestListDeploymentUsers(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			DeploymentUserList: []DeploymentUser{
				{
					ID:       "1",
					FullName: "some body",
					Emails: []Email{
						{Address: "somebody@astronomer.com"},
					},
					Username: "somebody",
					RoleBindings: []RoleBinding{
						{Role: DeploymentAdminRole},
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

		response, err := api.ListDeploymentUsers(ListDeploymentUsersRequest{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeploymentUserList)
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

		_, err := api.ListDeploymentUsers(ListDeploymentUsersRequest{})
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
		houstonMethodAvailabilityByVersion["ListDeploymentUsers"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListDeploymentUsers(ListDeploymentUsersRequest{})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListDeploymentUsers"})
	})
}

func TestAddDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			AddDeploymentUser: &RoleBinding{
				Role: DeploymentAdminRole,
				User: struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				}{
					ID:       "1",
					Username: "somebody",
				},
				ServiceAccount: WorkspaceServiceAccount{},
				Deployment: Deployment{
					ID: "deployment-id",
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

		response, err := api.AddDeploymentUser(UpdateDeploymentUserRequest{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.AddDeploymentUser)
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

		_, err := api.AddDeploymentUser(UpdateDeploymentUserRequest{})
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
		houstonMethodAvailabilityByVersion["AddDeploymentUser"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.AddDeploymentUser(UpdateDeploymentUserRequest{})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"AddDeploymentUser"})
	})
}

func TestUpdateDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			UpdateDeploymentUser: &RoleBinding{
				Role: DeploymentAdminRole,
				User: struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				}{
					ID:       "1",
					Username: "somebody",
				},
				ServiceAccount: WorkspaceServiceAccount{},
				Deployment: Deployment{
					ID: "deployment-id",
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

		response, err := api.UpdateDeploymentUser(UpdateDeploymentUserRequest{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.UpdateDeploymentUser)
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

		_, err := api.UpdateDeploymentUser(UpdateDeploymentUserRequest{})
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
		houstonMethodAvailabilityByVersion["UpdateDeploymentUser"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.UpdateDeploymentUser(UpdateDeploymentUserRequest{})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"UpdateDeploymentUser"})
	})
}

func TestDeleteDeploymentUser(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			DeleteDeploymentUser: &RoleBinding{
				Role: DeploymentAdminRole,
				User: struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				}{
					ID:       "1",
					Username: "somebody",
				},
				ServiceAccount: WorkspaceServiceAccount{},
				Deployment: Deployment{
					ID: "deployment-id",
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

		response, err := api.DeleteDeploymentUser(DeleteDeploymentUserRequest{"deployment-id", "email"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeleteDeploymentUser)
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

		_, err := api.DeleteDeploymentUser(DeleteDeploymentUserRequest{"deployment-id", "email"})
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
		houstonMethodAvailabilityByVersion["DeleteDeploymentUser"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteDeploymentUser(DeleteDeploymentUserRequest{"deployment-id", "email"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"DeleteDeploymentUser"})
	})
}
