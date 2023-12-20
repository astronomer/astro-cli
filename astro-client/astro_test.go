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

func TestCreateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.CreateDeployment(&CreateDeploymentInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.CreateDeployment(&CreateDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.UpdateDeployment(&UpdateDeploymentInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.UpdateDeployment(&UpdateDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeployments(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		deployments, err := astroClient.ListDeployments(org, "")
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

		_, err := astroClient.ListDeployments(org, "")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		deployments, err := astroClient.GetDeployment(deployment)
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeployment(deployment)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.DeleteDeployment(DeleteDeploymentInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.DeleteDeployment(DeleteDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeploymentHistory(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

func TestGetDeploymentConfigWithOrganization(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		deploymentConfig, err := astroClient.GetDeploymentConfigWithOrganization("test-org-id")
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

		_, err := astroClient.GetDeploymentConfigWithOrganization("test-org-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestModifyDeploymentVariable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		envVars, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestInitiateDagDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		envVars, err := astroClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestReportDagDeploymentStatus(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		image, err := astroClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCreateImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		image, err := astroClient.CreateImage(CreateImageInput{})
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

		_, err := astroClient.CreateImage(CreateImageInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeployImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		image, err := astroClient.DeployImage(&DeployImageInput{})
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

		_, err := astroClient.DeployImage(&DeployImageInput{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestWorkerQueueOptions(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		astroClient := NewAstroClient(client)

		resp, err := astroClient.GetWorkerQueueOptions()
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
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetWorkerQueueOptions()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetOrganizationAuditLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

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

func TestUpdateAlertEmails(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("updates a deployments alert emails", func(t *testing.T) {
		emails := []string{"test1@email.com", "test2@meail.com"}
		mockResponse := &Response{
			Data: ResponseData{
				DeploymentAlerts: DeploymentAlerts{AlertEmails: emails},
			},
			Errors: nil,
		}
		input := UpdateDeploymentAlertsInput{
			DeploymentID: "test-deployment-id",
			AlertEmails:  emails,
		}
		jsonResponse, err := json.Marshal(mockResponse)
		assert.NoError(t, err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		resp, err := astroClient.UpdateAlertEmails(input)
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResponse.Data.DeploymentAlerts)
	})
	t.Run("returns api error", func(t *testing.T) {
		emails := []string{"test1@email.com", "test2@meail.com"}
		input := UpdateDeploymentAlertsInput{
			DeploymentID: "test-deployment-id",
			AlertEmails:  emails,
		}
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.UpdateAlertEmails(input)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
