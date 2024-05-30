package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateDeploymentServiceAccount() {
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

		response, err := api.CreateDeploymentServiceAccount(&CreateServiceAccountRequest{})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.CreateDeploymentServiceAccount)
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

		_, err := api.CreateDeploymentServiceAccount(&CreateServiceAccountRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestCreateWorkspaceServiceAccount() {
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

		response, err := api.CreateWorkspaceServiceAccount(&CreateServiceAccountRequest{})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.CreateWorkspaceServiceAccount)
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

		_, err := api.CreateWorkspaceServiceAccount(&CreateServiceAccountRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteDeploymentServiceAccount() {
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

		response, err := api.DeleteDeploymentServiceAccount(DeleteServiceAccountRequest{"", "deployment-id", "sa-id"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeleteDeploymentServiceAccount)
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

		_, err := api.DeleteDeploymentServiceAccount(DeleteServiceAccountRequest{"", "deployment-id", "sa-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteWorkspaceServiceAccount() {
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

		response, err := api.DeleteWorkspaceServiceAccount(DeleteServiceAccountRequest{"workspace-id", "", "sa-id"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeleteWorkspaceServiceAccount)
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

		_, err := api.DeleteWorkspaceServiceAccount(DeleteServiceAccountRequest{"workspace-id", "", "sa-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListDeploymentServiceAccounts() {
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

		response, err := api.ListDeploymentServiceAccounts("deployment-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetDeploymentServiceAccounts)
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

		_, err := api.ListDeploymentServiceAccounts("deployment-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListWorkspaceServiceAccounts() {
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

		response, err := api.ListWorkspaceServiceAccounts("workspace-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetWorkspaceServiceAccounts)
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

		_, err := api.ListWorkspaceServiceAccounts("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}
