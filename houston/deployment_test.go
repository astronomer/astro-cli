package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestCreateDeployment(t *testing.T) {
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

		deployment, err := api.CreateDeployment(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.CreateDeployment)
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

		_, err := api.CreateDeployment(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestDeleteDeployment(t *testing.T) {
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

		deployment, err := api.DeleteDeployment(DeleteDeploymentRequest{"deployment-id", false})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.DeleteDeployment)
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

		_, err := api.DeleteDeployment(DeleteDeploymentRequest{"deployment-id", false})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeployments(t *testing.T) {
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

		deploymentList, err := api.ListDeployments(ListDeploymentsRequest{})
		assert.NoError(t, err)
		assert.Equal(t, deploymentList, mockDeploymentList.Data.GetDeployments)
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

		_, err := api.ListDeployments(ListDeploymentsRequest{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeployment(t *testing.T) {
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

		deployment, err := api.UpdateDeployment(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.UpdateDeployment)
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

		_, err := api.UpdateDeployment(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeployment(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockDeployment := &Response{
		Data: ResponseData{
			GetDeployment: []Deployment{
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
	jsonResponse, err := json.Marshal(mockDeployment)
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

		deployment, err := api.GetDeployment("deployment-id")
		assert.NoError(t, err)
		assert.Equal(t, deployment, &mockDeployment.Data.GetDeployment)
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

		_, err := api.GetDeployment("deployment-id")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeploymentAirflow(t *testing.T) {
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

		deployment, err := api.UpdateDeploymentAirflow(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.UpdateDeploymentAirflow)
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

		_, err := api.UpdateDeploymentAirflow(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetDeploymentConfig(t *testing.T) {
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

		deploymentConfig, err := api.GetDeploymentConfig(nil)
		assert.NoError(t, err)
		assert.Equal(t, *deploymentConfig, mockDeploymentConfig.Data.DeploymentConfig)
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

		_, err := api.GetDeploymentConfig(nil)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestListDeploymentLogs(t *testing.T) {
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

		logs, err := api.ListDeploymentLogs(ListDeploymentLogsRequest{})
		assert.NoError(t, err)
		assert.Equal(t, logs, mockDeployment.Data.DeploymentLog)
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

		_, err := api.ListDeploymentLogs(ListDeploymentLogsRequest{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeploymentRuntime(t *testing.T) {
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

		deployment, err := api.UpdateDeploymentRuntime(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.UpdateDeploymentRuntime)
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

		_, err := api.UpdateDeploymentRuntime(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestCancelUpdateDeploymentRuntime(t *testing.T) {
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

		deployment, err := api.CancelUpdateDeploymentRuntime(map[string]interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, deployment, mockDeployment.Data.CancelUpdateDeploymentRuntime)
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

		_, err := api.CancelUpdateDeploymentRuntime(map[string]interface{}{})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestUpdateDeploymentImage(t *testing.T) {
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, AirflowVersion: mockDeployment.Data.UpdateDeploymentImage.AirflowVersion})
		assert.NoError(t, err)
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

		_, err := api.UpdateDeploymentImage(UpdateDeploymentImageRequest{ReleaseName: mockDeployment.Data.UpdateDeploymentImage.ReleaseName, AirflowVersion: mockDeployment.Data.UpdateDeploymentImage.AirflowVersion})
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
