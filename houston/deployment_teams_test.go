package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestAddDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			AddDeploymentTeam: &RoleBinding{
				Role: DeploymentViewerRole,
				Team: Team{
					ID:   "test-id",
					Name: "test-team-name",
				},
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

		response, err := api.AddDeploymentTeam(AddDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.AddDeploymentTeam)
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

		_, err := api.AddDeploymentTeam(AddDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			RemoveDeploymentTeam: &RoleBinding{
				Role: DeploymentViewerRole,
				Team: Team{
					ID:   "test-id",
					Name: "test-team-name",
				},
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

		response, err := api.RemoveDeploymentTeam(RemoveDeploymentTeamRequest{"deployment-id", "team-id"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.RemoveDeploymentTeam)
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

		_, err := api.RemoveDeploymentTeam(RemoveDeploymentTeamRequest{"deployment-id", "team-id"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListDeploymentTeamsAndRoles() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := []Team{
		{
			ID: "test-id",
		},
	}
	jsonResponse, err := json.Marshal(Response{Data: ResponseData{
		DeploymentGetTeams: []Team{
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

		response, err := api.ListDeploymentTeamsAndRoles("deployment-id")
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

		_, err := api.ListDeploymentTeamsAndRoles("deploymeny-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeploymentTeamAndRole() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := &Response{
		Data: ResponseData{
			UpdateDeploymentTeam: &RoleBinding{
				Role: DeploymentAdminRole,
				Team: Team{
					ID:   "test-id",
					Name: "test-team-name",
				},
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

		response, err := api.UpdateDeploymentTeamRole(UpdateDeploymentTeamRequest{"deployment-id", "team-id", DeploymentAdminRole})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.UpdateDeploymentTeam)
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

		_, err := api.UpdateDeploymentTeamRole(UpdateDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
