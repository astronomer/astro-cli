package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			CreateDeployment: &Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "2.2.0",
				DesiredAirflowVersion: "2.2.0",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.CreateDeployment(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.CreateDeployment)
	})

	s.Run("success for upsert deployment", func() {
		localMockDeployment := &Response{
			Data: ResponseData{
				UpsertDeployment: &Deployment{
					ID:                    "deployment-test-id",
					Type:                  "airflow",
					Label:                 "test deployment",
					ReleaseName:           "prehistoric-gravity-930",
					Version:               "2.2.0",
					AirflowVersion:        "2.2.0",
					DesiredAirflowVersion: "2.2.0",
					DeploymentInfo:        DeploymentInfo{},
					Workspace: Workspace{
						ID: "test-workspace-id",
					},
					Urls: []DeploymentURL{
						{Type: "airflow", URL: "http://airflow.com"},
						{Type: "flower", URL: "http://flower.com"},
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
		}
		localJSONResponse, err := json.Marshal(localMockDeployment)
		s.NoError(err)

		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(localJSONResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		deployment, err := api.CreateDeployment(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, localMockDeployment.Data.UpsertDeployment)
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

		_, err := api.CreateDeployment(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestDeleteDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			DeleteDeployment: &Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "2.2.0",
				DesiredAirflowVersion: "2.2.0",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.DeleteDeployment(DeleteDeploymentRequest{"deployment-id", false})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.DeleteDeployment)
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

		_, err := api.DeleteDeployment(DeleteDeploymentRequest{"deployment-id", false})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListDeployments() {
	testUtil.InitTestConfig("software")

	mockDeploymentList := &Response{
		Data: ResponseData{
			GetDeployments: []Deployment{
				{
					ID:                    "deployment-test-id",
					Type:                  "airflow",
					Label:                 "test deployment",
					ReleaseName:           "prehistoric-gravity-930",
					Version:               "2.2.0",
					AirflowVersion:        "2.2.0",
					DesiredAirflowVersion: "2.2.0",
					DeploymentInfo:        DeploymentInfo{},
					Workspace: Workspace{
						ID: "test-workspace-id",
					},
					Urls: []DeploymentURL{
						{Type: "airflow", URL: "http://airflow.com"},
						{Type: "flower", URL: "http://flower.com"},
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeploymentList)
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

		deploymentList, err := api.ListDeployments(ListDeploymentsRequest{})
		s.NoError(err)
		s.Equal(deploymentList, mockDeploymentList.Data.GetDeployments)
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

		_, err := api.ListDeployments(ListDeploymentsRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			UpdateDeployment: &Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "2.2.0",
				DesiredAirflowVersion: "2.2.0",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.UpdateDeployment(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.UpdateDeployment)
	})

	s.Run("success for upsert deployment", func() {
		localMockDeployment := &Response{
			Data: ResponseData{
				UpsertDeployment: &Deployment{
					ID:                    "deployment-test-id",
					Type:                  "airflow",
					Label:                 "test deployment",
					ReleaseName:           "prehistoric-gravity-930",
					Version:               "2.2.0",
					AirflowVersion:        "2.2.0",
					DesiredAirflowVersion: "2.2.0",
					DeploymentInfo:        DeploymentInfo{},
					Workspace: Workspace{
						ID: "test-workspace-id",
					},
					Urls: []DeploymentURL{
						{Type: "airflow", URL: "http://airflow.com"},
						{Type: "flower", URL: "http://flower.com"},
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
		}
		localJSONResponse, err := json.Marshal(localMockDeployment)
		s.NoError(err)

		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(localJSONResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		deployment, err := api.UpdateDeployment(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, localMockDeployment.Data.UpsertDeployment)
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

		_, err := api.UpdateDeployment(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			GetDeployment: Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "2.2.0",
				DesiredAirflowVersion: "2.2.0",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.GetDeployment("deployment-id")
		s.NoError(err)
		s.Equal(deployment, &mockDeployment.Data.GetDeployment)
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

		_, err := api.GetDeployment("deployment-id")
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeploymentAirflow() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			UpdateDeploymentAirflow: &Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "2.2.0",
				DesiredAirflowVersion: "2.2.0",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.UpdateDeploymentAirflow(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.UpdateDeploymentAirflow)
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

		_, err := api.UpdateDeploymentAirflow(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetDeploymentConfig() {
	testUtil.InitTestConfig("software")

	mockDeploymentConfig := &Response{
		Data: ResponseData{
			DeploymentConfig: DeploymentConfig{
				AirflowImages: []AirflowImage{
					{Version: "1.1.0", Tag: "1.1.0"},
					{Version: "1.1.1", Tag: "1.1.1"},
					{Version: "1.1.2", Tag: "1.1.2"},
				},
				DefaultAirflowImageTag: "1.1.0",
				AirflowVersions: []string{
					"1.1.0",
					"1.1.2",
					"1.1.10",
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeploymentConfig)
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

		deploymentConfig, err := api.GetDeploymentConfig(nil)
		s.NoError(err)
		s.Equal(*deploymentConfig, mockDeploymentConfig.Data.DeploymentConfig)
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

		_, err := api.GetDeploymentConfig(nil)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestListDeploymentLogs() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			DeploymentLog: []DeploymentLog{
				{ID: "1", Component: "webserver", Log: "test1"},
				{ID: "2", Component: "scheduler", Log: "test2"},
				{ID: "3", Component: "webserver", Log: "test3"},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		logs, err := api.ListDeploymentLogs(ListDeploymentLogsRequest{})
		s.NoError(err)
		s.Equal(logs, mockDeployment.Data.DeploymentLog)
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

		_, err := api.ListDeploymentLogs(ListDeploymentLogsRequest{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeploymentRuntime() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &Response{
		Data: ResponseData{
			UpdateDeploymentRuntime: &Deployment{
				ID:                    "deployment-test-id",
				Type:                  "airflow",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				AirflowVersion:        "",
				DesiredAirflowVersion: "",
				RuntimeVersion:        "4.2.4",
				DesiredRuntimeVersion: "4.2.4",
				RuntimeAirflowVersion: "2.2.5",
				DeploymentInfo:        DeploymentInfo{},
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
				Urls: []DeploymentURL{
					{Type: "airflow", URL: "http://airflow.com"},
					{Type: "flower", URL: "http://flower.com"},
				},
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.UpdateDeploymentRuntime(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.UpdateDeploymentRuntime)
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

		_, err := api.UpdateDeploymentRuntime(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestCancelUpdateDeploymentRuntime() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &Response{
		Data: ResponseData{
			CancelUpdateDeploymentRuntime: &Deployment{
				ID:                    "deployment-test-id",
				Label:                 "test deployment",
				ReleaseName:           "prehistoric-gravity-930",
				Version:               "2.2.0",
				RuntimeVersion:        "4.2.4",
				DesiredRuntimeVersion: "4.2.4",
				RuntimeAirflowVersion: "2.2.5",
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.CancelUpdateDeploymentRuntime(map[string]interface{}{})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.CancelUpdateDeploymentRuntime)
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

		_, err := api.CancelUpdateDeploymentRuntime(map[string]interface{}{})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUpdateDeploymentImage() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &Response{
		Data: ResponseData{
			UpdateDeploymentImage: UpdateDeploymentImageResp{
				ReleaseName:    "prehistoric-gravity-930",
				AirflowVersion: "2.2.0",
				RuntimeVersion: "",
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, AirflowVersion: mockDeployment.Data.UpdateDeploymentImage.AirflowVersion})
		s.NoError(err)
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, AirflowVersion: mockDeployment.Data.UpdateDeploymentImage.AirflowVersion})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
