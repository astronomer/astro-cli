package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateWorkspace() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			CreateWorkspace: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "test description",
				Users: []User{
					{
						ID:       "id",
						Username: "test",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Status: "active",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.CreateWorkspace(CreateWorkspaceRequest{"label", "description"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.CreateWorkspace)
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

		_, err := api.CreateWorkspace(CreateWorkspaceRequest{"label", "description"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListWorkspaces() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetWorkspaces: []Workspace{
				{
					ID:          "workspace-id",
					Label:       "label",
					Description: "test description",
					Users: []User{
						{
							ID:       "id",
							Username: "test",
							Emails: []Email{
								{Address: "test@astronomer.com"},
							},
							Status: "active",
						},
					},
					CreatedAt: "2020-06-25T22:10:42.385Z",
					UpdatedAt: "2020-06-25T22:10:42.385Z",
				},
				{
					ID:          "workspace-id2",
					Label:       "label2",
					Description: "test description2",
					Users: []User{
						{
							ID:       "id",
							Username: "test",
							Emails: []Email{
								{Address: "test@astronomer.com"},
							},
							Status: "active",
						},
					},
					CreatedAt: "2020-06-25T22:10:42.385Z",
					UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.ListWorkspaces(nil)
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetWorkspaces)
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

		_, err := api.ListWorkspaces(nil)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestPaginatedListWorkspaces() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetPaginatedWorkspaces: []Workspace{
				{
					ID:          "workspace-id",
					Label:       "label",
					Description: "test description",
					Users: []User{
						{
							ID:       "id",
							Username: "test",
							Emails: []Email{
								{Address: "test@astronomer.com"},
							},
							Status: "active",
						},
					},
					CreatedAt: "2020-06-25T22:10:42.385Z",
					UpdatedAt: "2020-06-25T22:10:42.385Z",
				},
				{
					ID:          "workspace-id2",
					Label:       "label2",
					Description: "test description2",
					Users: []User{
						{
							ID:       "id",
							Username: "test",
							Emails: []Email{
								{Address: "test@astronomer.com"},
							},
							Status: "active",
						},
					},
					CreatedAt: "2020-06-25T22:10:42.385Z",
					UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.PaginatedListWorkspaces(PaginatedListWorkspaceRequest{10, 0})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetPaginatedWorkspaces)
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

		_, err := api.PaginatedListWorkspaces(PaginatedListWorkspaceRequest{10, 0})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteWorkspace() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			DeleteWorkspace: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "test description",
				Users: []User{
					{
						ID:       "id",
						Username: "test",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Status: "active",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.DeleteWorkspace("workspace-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.DeleteWorkspace)
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

		_, err := api.DeleteWorkspace("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetWorkspace() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetWorkspace: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "test description",
				Users: []User{
					{
						ID:       "id",
						Username: "test",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Status: "active",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.GetWorkspace("workspace-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetWorkspace)
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

		_, err := api.GetWorkspace("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestValidateWorkspaceID() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			GetWorkspace: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "test description",
				CreatedAt:   "2020-06-25T22:10:42.385Z",
				UpdatedAt:   "2020-06-25T22:10:42.385Z",
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

		response, err := api.ValidateWorkspaceID("workspace-id")
		s.NoError(err)
		s.Equal(response, mockResponse.Data.GetWorkspace)
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

		_, err := api.ValidateWorkspaceID("workspace-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateWorkspace() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			UpdateWorkspace: &Workspace{
				ID:          "workspace-id",
				Label:       "label",
				Description: "test description",
				Users: []User{
					{
						ID:       "id",
						Username: "test",
						Emails: []Email{
							{Address: "test@astronomer.com"},
						},
						Status: "active",
					},
				},
				CreatedAt: "2020-06-25T22:10:42.385Z",
				UpdatedAt: "2020-06-25T22:10:42.385Z",
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

		response, err := api.UpdateWorkspace(UpdateWorkspaceRequest{"workspace-id", map[string]string{}})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.UpdateWorkspace)
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

		_, err := api.UpdateWorkspace(UpdateWorkspaceRequest{"workspace-id", map[string]string{}})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
