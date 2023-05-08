package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestGetTeam() {
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

		response, err := api.GetTeam("team-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetTeam)
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

		_, err := api.GetTeam("team-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetTeamUsers() {
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

		response, err := api.GetTeamUsers("team-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetTeamUsers)
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

		_, err := api.GetTeamUsers("team-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListTeams() {
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

		response, err := api.ListTeams(ListTeamsRequest{"", 1})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.ListTeams)
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

		_, err := api.ListTeams(ListTeamsRequest{"", 1})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestCreateTeamSystemRoleBinding() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			CreateTeamSystemRoleBinding: RoleBinding{
				Role: SystemAdminRole,
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

		response, err := api.CreateTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.CreateTeamSystemRoleBinding.Role)
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

		_, err := api.CreateTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteTeamSystemRoleBinding() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeleteTeamSystemRoleBinding: RoleBinding{
				Role: SystemAdminRole,
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

		response, err := api.DeleteTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeleteTeamSystemRoleBinding.Role)
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

		_, err := api.DeleteTeamSystemRoleBinding(SystemRoleBindingRequest{"test-id", SystemAdminRole})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
