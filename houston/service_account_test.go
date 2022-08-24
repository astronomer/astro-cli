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

func TestCreateDeploymentServiceAccount(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			CreateDeploymentServiceAccount: &DeploymentServiceAccount{
				ID:             "id",
				APIKey:         "apikey",
				Label:          "test label",
				Category:       "test category",
				EntityType:     "DEPLOYMENT",
				DeploymentUUID: "deployment-id",
				LastUsedAt:     "2020-06-25T22:10:42.385Z",
				CreatedAt:      "2020-06-25T22:10:42.385Z",
				UpdatedAt:      "2020-06-25T22:10:42.385Z",
				Active:         true,
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

		response, err := api.CreateDeploymentServiceAccount(&CreateServiceAccountRequest{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.CreateDeploymentServiceAccount)
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

		_, err := api.CreateDeploymentServiceAccount(&CreateServiceAccountRequest{})
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
		houstonMethodAvailabilityByVersion["CreateDeploymentServiceAccount"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.CreateDeploymentServiceAccount(&CreateServiceAccountRequest{})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"CreateDeploymentServiceAccount"})
	})
}

func TestCreateWorkspaceServiceAccount(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			CreateWorkspaceServiceAccount: &WorkspaceServiceAccount{
				ID:            "id",
				APIKey:        "apikey",
				Label:         "test label",
				Category:      "test category",
				EntityType:    "DEPLOYMENT",
				WorkspaceUUID: "workspace-id",
				LastUsedAt:    "2020-06-25T22:10:42.385Z",
				CreatedAt:     "2020-06-25T22:10:42.385Z",
				UpdatedAt:     "2020-06-25T22:10:42.385Z",
				Active:        true,
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

		response, err := api.CreateWorkspaceServiceAccount(&CreateServiceAccountRequest{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.CreateWorkspaceServiceAccount)
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

		_, err := api.CreateWorkspaceServiceAccount(&CreateServiceAccountRequest{})
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
		houstonMethodAvailabilityByVersion["CreateWorkspaceServiceAccount"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.CreateWorkspaceServiceAccount(&CreateServiceAccountRequest{})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"CreateWorkspaceServiceAccount"})
	})
}

func TestDeleteDeploymentServiceAccount(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			DeleteDeploymentServiceAccount: &ServiceAccount{
				ID:         "id",
				APIKey:     "apikey",
				Label:      "test label",
				Category:   "test category",
				LastUsedAt: "2020-06-25T22:10:42.385Z",
				CreatedAt:  "2020-06-25T22:10:42.385Z",
				UpdatedAt:  "2020-06-25T22:10:42.385Z",
				Active:     true,
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

		response, err := api.DeleteDeploymentServiceAccount(DeleteServiceAccountRequest{"", "deployment-id", "sa-id"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeleteDeploymentServiceAccount)
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

		_, err := api.DeleteDeploymentServiceAccount(DeleteServiceAccountRequest{"", "deployment-id", "sa-id"})
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
		houstonMethodAvailabilityByVersion["DeleteDeploymentServiceAccount"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteDeploymentServiceAccount(DeleteServiceAccountRequest{"", "deployment-id", "sa-id"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"DeleteDeploymentServiceAccount"})
	})
}

func TestDeleteWorkspaceServiceAccount(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			DeleteWorkspaceServiceAccount: &ServiceAccount{
				ID:         "id",
				APIKey:     "apikey",
				Label:      "test label",
				Category:   "test category",
				LastUsedAt: "2020-06-25T22:10:42.385Z",
				CreatedAt:  "2020-06-25T22:10:42.385Z",
				UpdatedAt:  "2020-06-25T22:10:42.385Z",
				Active:     true,
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

		response, err := api.DeleteWorkspaceServiceAccount(DeleteServiceAccountRequest{"workspace-id", "", "sa-id"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeleteWorkspaceServiceAccount)
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

		_, err := api.DeleteWorkspaceServiceAccount(DeleteServiceAccountRequest{"workspace-id", "", "sa-id"})
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
		houstonMethodAvailabilityByVersion["DeleteWorkspaceServiceAccount"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteWorkspaceServiceAccount(DeleteServiceAccountRequest{"workspace-id", "", "sa-id"})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"DeleteWorkspaceServiceAccount"})
	})
}

func TestListDeploymentServiceAccounts(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetDeploymentServiceAccounts: []ServiceAccount{
				{
					ID:         "id",
					APIKey:     "apikey",
					Label:      "test label",
					Category:   "test category",
					LastUsedAt: "2020-06-25T22:10:42.385Z",
					CreatedAt:  "2020-06-25T22:10:42.385Z",
					UpdatedAt:  "2020-06-25T22:10:42.385Z",
					Active:     true,
				},
				{
					ID:         "id-2",
					APIKey:     "apikey-2",
					Label:      "test label2",
					Category:   "test category2",
					LastUsedAt: "2020-06-25T22:10:42.385Z",
					CreatedAt:  "2020-06-25T22:10:42.385Z",
					UpdatedAt:  "2020-06-25T22:10:42.385Z",
					Active:     false,
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

		response, err := api.ListDeploymentServiceAccounts("deployment-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetDeploymentServiceAccounts)
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

		_, err := api.ListDeploymentServiceAccounts("deployment-id")
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
		houstonMethodAvailabilityByVersion["ListDeploymentServiceAccounts"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListDeploymentServiceAccounts("deployment-id")
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListDeploymentServiceAccounts"})
	})
}

func TestListWorkspaceServiceAccounts(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetWorkspaceServiceAccounts: []ServiceAccount{
				{
					ID:         "id",
					APIKey:     "apikey",
					Label:      "test label",
					Category:   "test category",
					LastUsedAt: "2020-06-25T22:10:42.385Z",
					CreatedAt:  "2020-06-25T22:10:42.385Z",
					UpdatedAt:  "2020-06-25T22:10:42.385Z",
					Active:     true,
				},
				{
					ID:         "id-2",
					APIKey:     "apikey-2",
					Label:      "test label2",
					Category:   "test category2",
					LastUsedAt: "2020-06-25T22:10:42.385Z",
					CreatedAt:  "2020-06-25T22:10:42.385Z",
					UpdatedAt:  "2020-06-25T22:10:42.385Z",
					Active:     false,
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

		response, err := api.ListWorkspaceServiceAccounts("workspace-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetWorkspaceServiceAccounts)
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

		_, err := api.ListWorkspaceServiceAccounts("workspace-id")
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
		houstonMethodAvailabilityByVersion["ListWorkspaceServiceAccounts"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListWorkspaceServiceAccounts("workspace-id")
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListWorkspaceServiceAccounts"})
	})
}
