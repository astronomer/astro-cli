package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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

	s.Run("uses mode-aware query and passes mode for Houston >= 2.1.0", func() {
		oldVersion := version
		version = "2.1.0"
		defer func() { version = oldVersion }()

		var capturedBody string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.CreateDeployment(map[string]interface{}{"mode": OperatorDeploymentMode})
		s.NoError(err)
		s.Contains(capturedBody, "$mode: AllowedDeploymentModeValues")
		s.Contains(capturedBody, OperatorDeploymentMode)
	})

	s.Run("omits mode variable from query for Houston < 2.1.0", func() {
		oldVersion := version
		version = "1.0.1"
		defer func() { version = oldVersion }()

		var capturedBody string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.CreateDeployment(map[string]interface{}{})
		s.NoError(err)
		s.NotContains(capturedBody, "$mode: AllowedDeploymentModeValues")
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

func (s *Suite) TestAdoptDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			AdoptDeployment: &Deployment{
				ID:          "deployment-test-id",
				Label:       "test deployment",
				ReleaseName: "prod-airflow-4",
				Namespace:   "airflow-prod4",
				ClusterID:   "cluster-test-id",
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockDeployment)
	s.NoError(err)

	req := &AdoptDeploymentRequest{
		WorkspaceID:             "test-workspace-id",
		ClusterID:               "cluster-test-id",
		CRNamespace:             "airflow-prod4",
		CRName:                  "prod-airflow-4",
		AcceptIncompatibilities: true,
	}

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		deployment, err := api.AdoptDeployment(req)
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.AdoptDeployment)
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

		_, err := api.AdoptDeployment(req)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestUnadoptDeployment() {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			UnadoptDeployment: &Deployment{
				ID:          "deployment-test-id",
				Label:       "test deployment",
				ReleaseName: "prod-airflow-4",
				Namespace:   "airflow-prod4",
				ClusterID:   "cluster-test-id",
				Workspace: Workspace{
					ID: "test-workspace-id",
				},
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

		deployment, err := api.UnadoptDeployment(UnadoptDeploymentRequest{DeploymentID: "deployment-test-id"})
		s.NoError(err)
		s.Equal(deployment, mockDeployment.Data.UnadoptDeployment)
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

		_, err := api.UnadoptDeployment(UnadoptDeploymentRequest{DeploymentID: "deployment-test-id"})
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
		want := mockDeployment.Data.GetDeployment
		want.DagDeploymentRead = true
		s.Equal(&want, deployment)
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
				RuntimeVersion: "6.0.0",
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, RuntimeVersion: "6.0.0"})
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, RuntimeVersion: "6.0.0"})
		s.Contains(err.Error(), "Internal Server Error")
	})

	s.Run("Uses upsertDeployment API for Houston >= 2.1.0", func() {
		oldVersion := version
		version = "2.1.0"
		defer func() { version = oldVersion }()

		var capturedBody string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{
			ReleaseName:    mockDeployment.Data.UpdateDeploymentImage.ReleaseName,
			Image:          "registry.example.com/image:tag",
			RuntimeVersion: "6.0.0",
		})
		s.NoError(err)
		s.Contains(capturedBody, "upsertDeployment")
		s.NotContains(capturedBody, "updateDeploymentImage")
	})

	s.Run("Uses updateDeploymentImage API for Houston < 2.1.0", func() {
		oldVersion := version
		version = "1.0.1"
		defer func() { version = oldVersion }()

		var capturedBody string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{
			ReleaseName:    mockDeployment.Data.UpdateDeploymentImage.ReleaseName,
			Image:          "registry.example.com/image:tag",
			RuntimeVersion: "6.0.0",
		})
		s.NoError(err)
		s.Contains(capturedBody, "updateDeploymentImage")
		s.NotContains(capturedBody, "upsertDeployment")
	})
}

// The window goes only to a Houston that takes it: startTime and endTime came
// in 0.25.6. Before it the query declares neither and the variables carry
// neither.
func (s *Suite) TestListDeploymentLogsWindowByVersion() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	prevVersion := version
	defer func() { version = prevVersion }()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	for _, c := range []struct {
		houston    string
		withWindow bool
	}{
		{"0.25.0", false},
		{"0.25.5", false},
		{"0.25.6", true},
		{"2.1.0", true},
	} {
		version = c.houston
		var sent string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			b, _ := io.ReadAll(req.Body)
			sent = string(b)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"data":{"logs":[]}}`)), Header: make(http.Header)}
		})
		_, err := NewClient(client).ListDeploymentLogs(ListDeploymentLogsRequest{
			DeploymentID: "d", Timestamp: &at, LogWindow: &LogWindow{StartTime: at, EndTime: at.Add(time.Minute)},
		})
		s.NoError(err, c.houston)
		s.Equal(c.withWindow, strings.Contains(sent, `$startTime`), "query on %s", c.houston)
		s.Equal(c.withWindow, strings.Contains(sent, `"startTime":`), "variables on %s", c.houston)
	}
}

// GetDeployment asks for desiredRuntimeVersion only where Houston serves it:
// 0.29.0 up to 1.0.43, which removed it. Asked for after that, the whole
// query would fail validation.
func (s *Suite) TestGetDeploymentSelectsTheDesiredRuntimeVersionWhereServed() {
	for _, c := range []struct {
		houston string
		selects bool
	}{
		{"0.29.0", true},
		{"1.0.0", true},
		{"1.0.42", true},
		{"1.0.43", false},
		{"2.1.0", false},
	} {
		q := DeploymentGetRequest.GreatestLowerBound(c.houston)
		s.Equal(c.selects, strings.Contains(q, "desiredRuntimeVersion"), c.houston)
		if c.houston >= "1" {
			s.Contains(q, "workspace {", c.houston)
		}
	}
}

// GetDeployment says whether it read the Deployment's DAG deployment type,
// by the query it chose on the platform version it holds: from 0.29.0 on,
// and on a version it does not know, whose query is the newest. The type is
// read where it was asked for, not where a version says it would be.
func (s *Suite) TestGetDeploymentSaysWhetherItReadTheType() {
	testUtil.InitTestConfig("software")
	prev := version
	defer func() { version = prev }()
	for v, want := range map[string]bool{
		"0.25.0": false, "0.28.9": false,
		"0.29.0": true, "0.34.1": true, "1.0.0": true, "1.0.43": true, "2.1.0": true,
		"": true, "not-a-version": true,
	} {
		version = v
		var asked string
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			body, err := io.ReadAll(req.Body)
			s.NoError(err)
			asked = string(body)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"data":{"deployment":{"id":"d"}}}`)), Header: make(http.Header)}
		})
		deployment, err := NewClient(client).GetDeployment("d")
		s.Require().NoError(err, v)
		s.Equal(want, deployment.DagDeploymentRead, v)
		s.Equal(want, strings.Contains(asked, "dagDeployment"), v)
	}
}
