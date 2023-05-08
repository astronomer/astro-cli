package astro

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestListWorkspaces() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		workspaces, err := astroClient.ListWorkspaces("organization-id")
		s.NoError(err)
		s.Equal(workspaces, mockResponse.Data.GetWorkspaces)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListWorkspaces("organization-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestCreateDeployment() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.CreateDeployment(&CreateDeploymentInput{})
		s.NoError(err)
		s.Equal(deployment, mockResponse.Data.CreateDeployment)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.CreateDeployment(&CreateDeploymentInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeployment() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.UpdateDeployment(&UpdateDeploymentInput{})
		s.NoError(err)
		s.Equal(deployment, mockResponse.Data.UpdateDeployment)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.UpdateDeployment(&UpdateDeploymentInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListDeployments() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deployments, err := astroClient.ListDeployments(org, "")
		s.NoError(err)
		s.Equal(deployments, mockResponse.Data.GetDeployments)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListDeployments(org, "")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetDeployment() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deployments, err := astroClient.GetDeployment(deployment)
		s.NoError(err)
		s.Equal(deployments, mockResponse.Data.GetDeployment)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeployment(deployment)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteDeployment() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deployment, err := astroClient.DeleteDeployment(DeleteDeploymentInput{})
		s.NoError(err)
		s.Equal(deployment, mockResponse.Data.DeleteDeployment)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.DeleteDeployment(DeleteDeploymentInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetDeploymentHistory() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deploymentHistory, err := astroClient.GetDeploymentHistory(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deploymentHistory, mockResponse.Data.GetDeploymentHistory)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeploymentHistory(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetDeploymentConfig() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		deploymentConfig, err := astroClient.GetDeploymentConfig()
		s.NoError(err)
		s.Equal(deploymentConfig, mockResponse.Data.GetDeploymentConfig)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetDeploymentConfig()
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestModifyDeploymentVariable() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		envVars, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		s.NoError(err)
		s.Equal(envVars, mockResponse.Data.UpdateDeploymentVariables)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ModifyDeploymentVariable(EnvironmentVariablesInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestInitiateDagDeployment() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		envVars, err := astroClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
		s.NoError(err)
		s.Equal(envVars, mockResponse.Data.InitiateDagDeployment)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.InitiateDagDeployment(InitiateDagDeploymentInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestReportDagDeploymentStatus() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		image, err := astroClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
		s.NoError(err)
		s.Equal(image, mockResponse.Data.ReportDagDeploymentStatus)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ReportDagDeploymentStatus(&ReportDagDeploymentStatusInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestCreateImage() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		image, err := astroClient.CreateImage(CreateImageInput{})
		s.NoError(err)
		s.Equal(image, mockResponse.Data.CreateImage)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.CreateImage(CreateImageInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeployImage() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		image, err := astroClient.DeployImage(DeployImageInput{})
		s.NoError(err)
		s.Equal(image, mockResponse.Data.DeployImage)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.DeployImage(DeployImageInput{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListClusters() {
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
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		resp, err := astroClient.ListClusters("test-org-id")
		s.NoError(err)
		s.Equal(resp, mockResponse.Data.GetClusters)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.ListClusters("test-org-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetWorkspace() {
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
	s.NoError(err)

	s.Run("happy path", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		workspace, err := astroClient.GetWorkspace("test-workspace")
		s.NoError(err)
		s.Equal(workspace, expectedWorkspace.Data.GetWorkspace)
	})
	s.Run("error path", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)
		_, err := astroClient.GetWorkspace("test-workspace")
		s.Error(err, "API error")
	})
}

func (s *Suite) TestWorkerQueueOptions() {
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
	s.NoError(err)

	s.Run("happy path", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		resp, err := astroClient.GetWorkerQueueOptions()
		s.NoError(err)
		s.Equal(resp, mockResponse.Data.GetWorkerQueueOptions)
	})

	s.Run("error path", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err := astroClient.GetWorkerQueueOptions()
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetOrganizationAuditLogs() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("Can export organization audit logs", func() {
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
		s.NoError(err)
		output := new(bytes.Buffer)
		io.Copy(output, resp)
		s.Equal(mockResponse, output.String())
	})

	s.Run("Permission denied", func() {
		errorMessage := "Invalid authorization token."
		errorResponse, err := json.Marshal(map[string]string{"message": errorMessage})
		s.NoError(err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(bytes.NewBuffer(errorResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err = astroClient.GetOrganizationAuditLogs("test-org-id", 50)
		s.Contains(err.Error(), errorMessage)
	})

	s.Run("Invalid earliest parameter", func() {
		errorMessage := "Invalid query parameter values: field earliest failed check on min;"
		errorResponse, err := json.Marshal(map[string]string{"message": errorMessage})
		s.NoError(err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBuffer(errorResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		_, err = astroClient.GetOrganizationAuditLogs("test-org-id", 50)
		s.Contains(err.Error(), errorMessage)
	})
}

func (s *Suite) TestUpdateAlertEmails() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("updates a deployments alert emails", func() {
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
		s.NoError(err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		astroClient := NewAstroClient(client)

		resp, err := astroClient.UpdateAlertEmails(input)
		s.NoError(err)
		s.Equal(resp, mockResponse.Data.DeploymentAlerts)
	})
	s.Run("returns api error", func() {
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
		s.Contains(err.Error(), "Internal Server Error")
	})
}
