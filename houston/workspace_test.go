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

func TestCreateWorkspace(t *testing.T) {
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

		response, err := api.CreateWorkspace("label", "description")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.CreateWorkspace)
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

		_, err := api.CreateWorkspace("label", "description")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListWorkspaces(t *testing.T) {
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

		response, err := api.ListWorkspaces()
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetWorkspaces)
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

		_, err := api.ListWorkspaces()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteWorkspace(t *testing.T) {
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

		response, err := api.DeleteWorkspace("workspace-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.DeleteWorkspace)
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

		_, err := api.DeleteWorkspace("workspace-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetWorkspace(t *testing.T) {
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

		response, err := api.GetWorkspace("workspace-id")
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.GetWorkspace)
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

		_, err := api.GetWorkspace("workspace-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateWorkspace(t *testing.T) {
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

		response, err := api.UpdateWorkspace("workspace-id", map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, response, mockResponse.Data.UpdateWorkspace)
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

		_, err := api.UpdateWorkspace("workspace-id", map[string]string{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
