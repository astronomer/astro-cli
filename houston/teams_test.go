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

func TestGetTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetTeam: &Team{
				Name: "Everyone",
				ID:   "blah-id",
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

		response, err := api.GetTeam("team-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetTeam)
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

		_, err := api.GetTeam("team-id")
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
		houstonMethodAvailabilityByVersion["GetTeam"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetTeam("team-id")
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"GetTeam"})
	})
}

func TestGetTeamUsers(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetTeamUsers: []User{
				{
					Username: "email@email.com",
					ID:       "test-id",
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

		response, err := api.GetTeamUsers("team-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetTeamUsers)
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

		_, err := api.GetTeamUsers("team-id")
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
		houstonMethodAvailabilityByVersion["GetTeamUsers"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetTeamUsers("team-id")
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"GetTeamUsers"})
	})
}

func TestListTeams(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			ListTeams: ListTeamsResp{
				Count: 1,
				Teams: []Team{
					{
						ID:   "test-id",
						Name: "test-name",
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

		response, err := api.ListTeams(ListTeamsRequest{"", 1})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.ListTeams)
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

		_, err := api.ListTeams(ListTeamsRequest{"", 1})
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
		houstonMethodAvailabilityByVersion["ListTeams"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.ListTeams(ListTeamsRequest{"", 1})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"ListTeams"})
	})
}

func TestCreateTeamSystemRoleBinding(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			CreateTeamSystemRoleBinding: RoleBinding{
				Role: SystemAdminRole,
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

		response, err := api.CreateTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.CreateTeamSystemRoleBinding.Role)
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

		_, err := api.CreateTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
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
		houstonMethodAvailabilityByVersion["CreateTeamSystemRoleBinding"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.CreateTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"CreateTeamSystemRoleBinding"})
	})
}

func TestDeleteTeamSystemRoleBinding(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeleteTeamSystemRoleBinding: RoleBinding{
				Role: SystemAdminRole,
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

		response, err := api.DeleteTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeleteTeamSystemRoleBinding.Role)
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

		_, err := api.DeleteTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
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
		houstonMethodAvailabilityByVersion["DeleteTeamSystemRoleBinding"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.DeleteTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"DeleteTeamSystemRoleBinding"})
	})
}
