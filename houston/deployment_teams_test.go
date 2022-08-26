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

func TestAddDeploymentTeam(t *testing.T) {
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

		response, err := api.AddDeploymentTeam(AddDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.AddDeploymentTeam)
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

		_, err := api.AddDeploymentTeam(AddDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteDeploymentTeam(t *testing.T) {
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

		response, err := api.RemoveDeploymentTeam(RemoveDeploymentTeamRequest{"deployment-id", "team-id"})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.RemoveDeploymentTeam)
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

		_, err := api.RemoveDeploymentTeam(RemoveDeploymentTeamRequest{"deployment-id", "team-id"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeploymentTeamsAndRoles(t *testing.T) {
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

		response, err := api.ListDeploymentTeamsAndRoles("deployment-id")
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

		_, err := api.ListDeploymentTeamsAndRoles("deploymeny-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeploymentTeamAndRole(t *testing.T) {
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

		response, err := api.UpdateDeploymentTeamRole(UpdateDeploymentTeamRequest{"deployment-id", "team-id", DeploymentAdminRole})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.UpdateDeploymentTeam)
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

		_, err := api.UpdateDeploymentTeamRole(UpdateDeploymentTeamRequest{"deployment-id", "team-id", "role"})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
