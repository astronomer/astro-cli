package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestListDeploymentUsers() {
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

		response, err := api.ListDeploymentUsers(ListDeploymentUsersRequest{})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeploymentUserList)
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

		_, err := api.ListDeploymentUsers(ListDeploymentUsersRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestAddDeploymentUser() {
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

		response, err := api.AddDeploymentUser(UpdateDeploymentUserRequest{})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.AddDeploymentUser)
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

		_, err := api.AddDeploymentUser(UpdateDeploymentUserRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeploymentUser() {
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

		response, err := api.UpdateDeploymentUser(UpdateDeploymentUserRequest{})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.UpdateDeploymentUser)
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

		_, err := api.UpdateDeploymentUser(UpdateDeploymentUserRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteDeploymentUser() {
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

		response, err := api.DeleteDeploymentUser(DeleteDeploymentUserRequest{"deployment-id", "email"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeleteDeploymentUser)
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

		_, err := api.DeleteDeploymentUser(DeleteDeploymentUserRequest{"deployment-id", "email"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
