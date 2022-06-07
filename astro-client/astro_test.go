package astro

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestListUserRoleBindings(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			SelfQuery: &Self{
				User: User{
					RoleBindings: []RoleBinding{
						{
							Role: "test-role",
							User: struct {
								ID       string `json:"id"`
								Username string `json:"username"`
							}{
								ID:       "test-user-id",
								Username: "test-user-name",
							},
							Deployment: Deployment{
								ID: "test-deployment-id",
							},
						},
					},
				},
			},
		},
	}

	mockInvalidUserResponse := &Response{
		Data: ResponseData{
			SelfQuery: nil,
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	invalidUserJSONResponse, err := json.Marshal(mockInvalidUserResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		roleBindings, err := astroClient.ListUserRoleBindings()
		assert.NoError(t, err)
		assert.Equal(t, roleBindings, mockResponse.Data.SelfQuery.User.RoleBindings)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListUserRoleBindings()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("invalid user", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(invalidUserJSONResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)
		_, err := astroClient.ListUserRoleBindings()
		assert.Contains(t, err.Error(), "something went wrong! Try again or contact Astronomer Support")
	})
}

func TestListWorkspaces(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetWorkspaces: []Workspace{
				{
					ID:           "test-id",
					Label:        "test-label",
					Users:        []User{},
					RoleBindings: []RoleBinding{},
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
		astroClient := NewAstroClient(client)

		workspaces, err := astroClient.ListWorkspaces()
		assert.NoError(t, err)
		assert.Equal(t, workspaces, mockResponse.Data.GetWorkspaces)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListWorkspaces()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCreateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeploymentCreate: Deployment{
				ID:             "test-deployment-id",
				Label:          "test-deployment-label",
				ReleaseName:    "test-release-name",
				RuntimeRelease: RuntimeRelease{Version: "4.2.5"},
				Workspace:      Workspace{ID: "test-workspace-id"},
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.CreateDeployment(&DeploymentCreateInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.DeploymentCreate)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.CreateDeployment(&DeploymentCreateInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeploymentUpdate: Deployment{
				ID:             "test-deployment-id",
				Label:          "test-deployment-label",
				ReleaseName:    "test-release-name",
				RuntimeRelease: RuntimeRelease{Version: "4.2.5"},
				Workspace:      Workspace{ID: "test-workspace-id"},
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.UpdateDeployment(&DeploymentUpdateInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.DeploymentUpdate)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.UpdateDeployment(&DeploymentUpdateInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeployments(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetDeployments: []Deployment{
				{
					ID:             "test-deployment-id",
					Label:          "test-deployment-label",
					ReleaseName:    "test-release-name",
					RuntimeRelease: RuntimeRelease{Version: "4.2.5"},
					Workspace:      Workspace{ID: "test-workspace-id"},
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
		astroClient := NewAstroClient(client)

		deployments, err := astroClient.ListDeployments(DeploymentsInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployments, mockResponse.Data.GetDeployments)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListDeployments(DeploymentsInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeploymentDelete: Deployment{
				ID:             "test-deployment-id",
				Label:          "test-deployment-label",
				ReleaseName:    "test-release-name",
				RuntimeRelease: RuntimeRelease{Version: "4.2.5"},
				Workspace:      Workspace{ID: "test-workspace-id"},
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.DeleteDeployment(DeploymentDeleteInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.DeploymentDelete)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.DeleteDeployment(DeploymentDeleteInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeploymentHistory(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetDeploymentHistory: DeploymentHistory{
				DeploymentID: "test-deployment-id",
				ReleaseName:  "test-release-name",
				SchedulerLogs: []SchedulerLog{
					{
						Timestamp: "2020-06-25T22:10:42.385Z",
						Raw:       "test-log-line",
						Level:     "info",
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
		astroClient := NewAstroClient(client)

		deploymentHistory, err := astroClient.GetDeploymentHistory(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deploymentHistory, mockResponse.Data.GetDeploymentHistory)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeploymentHistory(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeploymentConfig(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetDeploymentConfig: DeploymentConfig{
				AstronomerUnit: AstronomerUnit{CPU: 1, Memory: 1024},
				RuntimeReleases: []RuntimeRelease{
					{
						Version:                  "4.2.5",
						AirflowVersion:           "2.2.5",
						Channel:                  "stable",
						ReleaseDate:              "2020-06-25",
						AirflowDatabaseMigration: true,
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
		astroClient := NewAstroClient(client)

		deploymentConfig, err := astroClient.GetDeploymentConfig()
		assert.NoError(t, err)
		assert.Equal(t, deploymentConfig, mockResponse.Data.GetDeploymentConfig)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeploymentConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestModifyDeploymentVariable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeploymentVariablesUpdate: []EnvironmentVariablesObject{
				{
					Key:      "test-env-key",
					Value:    "test-env-value",
					IsSecret: false,
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
		astroClient := NewAstroClient(client)

		envVars, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		assert.NoError(t, err)
		assert.Equal(t, envVars, mockResponse.Data.DeploymentVariablesUpdate)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCreateImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			CreateImage: &Image{
				ID:           "test-image-id",
				DeploymentID: "test-deployment-id",
				Env:          []string{"test-env"},
				Labels:       []string{"test-label"},
				Name:         "test-image-name",
				Tag:          "test-tag",
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
		astroClient := NewAstroClient(client)

		image, err := astroClient.CreateImage(ImageCreateInput{})
		assert.NoError(t, err)
		assert.Equal(t, image, mockResponse.Data.CreateImage)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.CreateImage(ImageCreateInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeployImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeployImage: &Image{
				ID:           "test-image-id",
				DeploymentID: "test-deployment-id",
				Env:          []string{"test-env"},
				Labels:       []string{"test-label"},
				Name:         "test-image-name",
				Tag:          "test-tag",
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
		astroClient := NewAstroClient(client)

		image, err := astroClient.DeployImage(ImageDeployInput{})
		assert.NoError(t, err)
		assert.Equal(t, image, mockResponse.Data.DeployImage)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.DeployImage(ImageDeployInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListClusters(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetClusters: []Cluster{
				{
					ID:            "test-id",
					Name:          "test-name",
					IsManaged:     false,
					CloudProvider: "test-cloud-provider",
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
		astroClient := NewAstroClient(client)

		resp, err := astroClient.ListClusters(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.GetClusters)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListClusters(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListInternalRuntimeReleases(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			RuntimeReleases: []RuntimeRelease{
				{
					Version:                  "4.2.5",
					AirflowVersion:           "2.2.5",
					Channel:                  "stable",
					ReleaseDate:              "2020-06-25",
					AirflowDatabaseMigration: true,
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
		astroClient := NewAstroClient(client)

		resp, err := astroClient.ListInternalRuntimeReleases()
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.RuntimeReleases)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListInternalRuntimeReleases()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListPublicRuntimeReleases(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			RuntimeReleases: []RuntimeRelease{
				{
					Version:                  "4.2.5",
					AirflowVersion:           "2.2.5",
					Channel:                  "stable",
					ReleaseDate:              "2020-06-25",
					AirflowDatabaseMigration: true,
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
		astroClient := NewAstroClient(client)

		resp, err := astroClient.ListPublicRuntimeReleases()
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.RuntimeReleases)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListPublicRuntimeReleases()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
