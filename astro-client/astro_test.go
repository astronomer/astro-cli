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

func TestGetUserInfo(t *testing.T) {
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
				AuthenticatedOrganizationID: "test-org-id",
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
		gqlClient := NewGQLClient(client)

		roleBindings, err := gqlClient.GetUserInfo()
		assert.NoError(t, err)
		assert.Equal(t, roleBindings, mockResponse.Data.SelfQuery)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetUserInfo()
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
		gqlClient := NewGQLClient(client)
		_, err := gqlClient.GetUserInfo()
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
		gqlClient := NewGQLClient(client)

		workspaces, err := gqlClient.ListWorkspaces("organization-id")
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.ListWorkspaces("organization-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCreateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			CreateDeployment: Deployment{
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
		gqlClient := NewGQLClient(client)

		deployment, err := gqlClient.CreateDeployment(&CreateDeploymentInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.CreateDeployment)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.CreateDeployment(&CreateDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			UpdateDeployment: Deployment{
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
		gqlClient := NewGQLClient(client)

		deployment, err := gqlClient.UpdateDeployment(&UpdateDeploymentInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.UpdateDeployment)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.UpdateDeployment(&UpdateDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeployments(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	org := "test-org-id"
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
		gqlClient := NewGQLClient(client)

		deployments, err := gqlClient.ListDeployments(org, "")
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.ListDeployments(org, "")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	deployment := "test-deployment-id"
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
		gqlClient := NewGQLClient(client)

		deployments, err := gqlClient.GetDeployment(deployment)
		assert.NoError(t, err)
		assert.Equal(t, deployments, mockResponse.Data.GetDeployment)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetDeployment(deployment)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			DeleteDeployment: Deployment{
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
		gqlClient := NewGQLClient(client)

		deployment, err := gqlClient.DeleteDeployment(DeleteDeploymentInput{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockResponse.Data.DeleteDeployment)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.DeleteDeployment(DeleteDeploymentInput{})
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
		gqlClient := NewGQLClient(client)

		deploymentHistory, err := gqlClient.GetDeploymentHistory(map[string]interface{}{})
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetDeploymentHistory(map[string]interface{}{})
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
		gqlClient := NewGQLClient(client)

		deploymentConfig, err := gqlClient.GetDeploymentConfig()
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetDeploymentConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestModifyDeploymentVariable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			UpdateDeploymentVariables: []EnvironmentVariablesObject{
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
		gqlClient := NewGQLClient(client)

		envVars, err := gqlClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		assert.NoError(t, err)
		assert.Equal(t, envVars, mockResponse.Data.UpdateDeploymentVariables)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestInitiateDagDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			InitiateDagDeployment: InitiateDagDeployment{
				ID:     "test-id",
				DagURL: "test-url",
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
		gqlClient := NewGQLClient(client)

		envVars, err := gqlClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
		assert.NoError(t, err)
		assert.Equal(t, envVars, mockResponse.Data.InitiateDagDeployment)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestReportDagDeploymentStatus(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			ReportDagDeploymentStatus: DagDeploymentStatus{
				ID:            "test-id",
				RuntimeID:     "test-runtime-id",
				Action:        "test-action",
				VersionID:     "test-version-id",
				Status:        "test-status",
				Message:       "test-message",
				CreatedAt:     "test-created-at",
				InitiatorID:   "test-initiator-id",
				InitiatorType: "test-initiator-type",
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
		gqlClient := NewGQLClient(client)

		image, err := gqlClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
		assert.NoError(t, err)
		assert.Equal(t, image, mockResponse.Data.ReportDagDeploymentStatus)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
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
		gqlClient := NewGQLClient(client)

		image, err := gqlClient.CreateImage(CreateImageInput{})
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.CreateImage(CreateImageInput{})
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
		gqlClient := NewGQLClient(client)

		image, err := gqlClient.DeployImage(DeployImageInput{})
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.DeployImage(DeployImageInput{})
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
		gqlClient := NewGQLClient(client)

		resp, err := gqlClient.ListClusters("test-org-id")
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
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.ListClusters("test-org-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCreateUserInvite(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testInput := CreateUserInviteInput{
		InviteeEmail:   "test@email.com",
		Role:           "ORGANIZATION_MEMBER",
		OrganizationID: "test-org-id",
	}
	testInputWithInvalidEmail := CreateUserInviteInput{
		InviteeEmail:   "invalid-email",
		Role:           "ORGANIZATION_MEMBER",
		OrganizationID: "test-org-id",
	}
	mockResponse := &Response{
		Data: ResponseData{
			CreateUserInvite: UserInvite{
				UserID:         "test-user-id",
				OrganizationID: "test-org-id",
				OauthInviteID:  "test-oauth-invite-id",
				ExpiresAt:      "now+10mins",
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
		gqlClient := NewGQLClient(client)

		resp, err := gqlClient.CreateUserInvite(testInput)
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.CreateUserInvite)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.CreateUserInvite(testInputWithInvalidEmail)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetWorkspace(t *testing.T) {
	expectedWorkspace := Response{
		Data: ResponseData{
			GetWorkspace: Workspace{
				ID:             "",
				Label:          "",
				OrganizationID: "",
			},
		},
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	jsonResponse, err := json.Marshal(expectedWorkspace)
	assert.NoError(t, err)

	t.Run("happy path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		workspace, err := gqlClient.GetWorkspace("test-workspace")
		assert.NoError(t, err)
		assert.Equal(t, workspace, expectedWorkspace.Data.GetWorkspace)
	})
	t.Run("error path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)
		_, err := gqlClient.GetWorkspace("test-workspace")
		assert.Error(t, err, "API error")
	})
}

func TestWorkerQueueOptions(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := Response{
		Data: ResponseData{
			GetWorkerQueueOptions: WorkerQueueDefaultOptions{
				MinWorkerCount: WorkerQueueOption{
					Floor:   1,
					Ceiling: 10,
					Default: 2,
				},
				MaxWorkerCount: WorkerQueueOption{
					Floor:   2,
					Ceiling: 20,
					Default: 5,
				},
				WorkerConcurrency: WorkerQueueOption{
					Floor:   5,
					Ceiling: 80,
					Default: 25,
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("happy path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		resp, err := gqlClient.GetWorkerQueueOptions()
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.GetWorkerQueueOptions)
	})

	t.Run("error path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetWorkerQueueOptions()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetOrganizations(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := &Response{
		Data: ResponseData{
			GetOrganizations: []Organization{
				{
					ID:   "test-org-id",
					Name: "test-org-name",
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("happy path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		resp, err := gqlClient.GetOrganizations()
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.GetOrganizations)
	})

	t.Run("error path", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		gqlClient := NewGQLClient(client)

		_, err := gqlClient.GetOrganizations()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetOrganizationAuditLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("Can export organization audit logs", func(t *testing.T) {
		mockResponse := "A lot of audit logs entries"
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer([]byte(mockResponse))),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		resp, err := astroClient.GetOrganizationAuditLogs("test-org-id", 50)
		assert.NoError(t, err)
		output := new(bytes.Buffer)
		io.Copy(output, resp)
		assert.Equal(t, mockResponse, output.String())
	})

	t.Run("Permission denied", func(t *testing.T) {
		errorMessage := "Invalid authorization token."
		errorResponse, err := json.Marshal(map[string]string{"message": errorMessage})
		assert.NoError(t, err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(bytes.NewBuffer(errorResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err = astroClient.GetOrganizationAuditLogs("test-org-id", 50)
		assert.Contains(t, err.Error(), errorMessage)
	})

	t.Run("Invalid earliest parameter", func(t *testing.T) {
		errorMessage := "Invalid query parameter values: field earliest failed check on min;"
		errorResponse, err := json.Marshal(map[string]string{"message": errorMessage})
		assert.NoError(t, err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBuffer(errorResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err = astroClient.GetOrganizationAuditLogs("test-org-id", 50)
		assert.Contains(t, err.Error(), errorMessage)
	})
}
