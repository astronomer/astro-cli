package airflowclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestNewAirflowClient(t *testing.T) {
	client := NewAirflowClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro client")
}

func TestDoAirflowClient(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockResponse := "{\"connections\":[{\"connection_id\":\"\"}]}"
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer([]byte(mockResponse))),
			Header:     make(http.Header),
		}
	})
	airflowClient := NewAirflowClient(client)
	doOpts := &httputil.DoOptions{
		Path: "/test",
		Headers: map[string]string{
			"test": "test",
		},
	}
	_, err := airflowClient.DoAirflowClient(doOpts)
	assert.NoError(t, err)
}

var mockConnResponse = &Response{
	Connections: []Connection{
		{
			ConnID:      "test-id",
			ConnType:    "test-type",
			Description: "test-description",
			Host:        "test-host",
			Port:        1234,
		},
	},
}

var mockConn = &Connection{
	ConnID:      "test-id",
	ConnType:    "test-type",
	Description: "test-description",
	Host:        "test-host",
	Port:        1234,
}

var mockVarResponse = &Response{
	Variables: []Variable{
		{
			Description: "test-description",
			Key:         "test-key",
			Value:       "test-value",
		},
	},
}

var mockVar = &Variable{
	Description: "test-description",
	Key:         "test-key",
	Value:       "test-value",
}

func TestGetConnections(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	jsonResponse, err := json.Marshal(mockConnResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "GET", req.Method)
			assert.Equal(t, "https://test-airflow-url/api/v1/connections", req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetConnections("test-airflow-url")
		assert.NoError(t, err)
		assert.Equal(t, response, *mockConnResponse)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetConnections("test-airflow-url")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})
	t.Run("error - failed to decode response", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("Invalid JSON Response")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetConnections("test-airflow-url")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})
}

func TestUpdateConnection(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	connJSON, err := json.Marshal(mockConn)
	assert.NoError(t, err)
	jsonResponse, err := json.Marshal(mockConnResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "PATCH", req.Method)
			expectedURL := fmt.Sprintf("https://test-airflow-url/api/v1/connections/%s", mockConn.ConnID)
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, connJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal connection to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		// Pass a nil connection to force JSON marshal error
		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})

	t.Run("error - failed to execute HTTP request", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (400): Bad Request")
	})
}

func TestCreateConnection(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	connJSON, err := json.Marshal(mockConn)
	assert.NoError(t, err)
	jsonResponse, err := json.Marshal(mockConnResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "POST", req.Method)
			expectedURL := "https://test-airflow-url/api/v1/connections"
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, connJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal connection to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		// Pass a nil connection to force JSON marshal error
		err := airflowClient.CreateConnection("test-airflow-url", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})

	t.Run("error - failed to execute HTTP request", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (400): Bad Request")
	})
}

func TestCreateVariable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedURL := "https://test-airflow-url/api/v1/variables"

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			expectedReqBody, err := json.Marshal(mockVar)
			assert.NoError(t, err)
			assert.Equal(t, expectedReqBody, reqBody)

			responseJSON, err := json.Marshal(mockVarResponse)
			assert.NoError(t, err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", *mockVar)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", *mockVar)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal variable to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", Variable{Key: "", Value: "test-value"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})
}

func TestGetVariables(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedURL := "https://test-airflow-url/api/v1/variables"

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "GET", req.Method)
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			responseJSON, err := json.Marshal(mockVarResponse)
			assert.NoError(t, err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetVariables("test-airflow-url")
		assert.NoError(t, err)
		assert.Equal(t, mockVarResponse, &response)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetVariables("test-airflow-url")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})
}

func TestUpdateVariable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedURL := "https://test-airflow-url/api/v1/variables/test-key"

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "PATCH", req.Method)
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			expectedReqBody, err := json.Marshal(mockVar)
			assert.NoError(t, err)
			assert.Equal(t, expectedReqBody, reqBody)

			responseJSON, err := json.Marshal(mockVarResponse)
			assert.NoError(t, err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", *mockVar)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", *mockVar)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal variable to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", Variable{Key: "", Value: "test-value"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})
}

var mockPoolResponse = &Response{
	Pools: []Pool{
		{
			Description: "test-description",
			Name:        "test-name",
			Slots:       5,
		},
	},
}

var mockPool = &Pool{
	Description: "test-description",
	Name:        "test-name",
	Slots:       5,
}

func TestCreatePool(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	poolJSON, err := json.Marshal(mockPool)
	assert.NoError(t, err)
	jsonResponse, err := json.Marshal(mockPoolResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "POST", req.Method)
			expectedURL := "https://test-airflow-url/api/v1/pools"
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, poolJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal pool to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		// Pass a nil pool to force JSON marshal error
		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})

	t.Run("error - failed to execute HTTP request", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (400): Bad Request")
	})
}

func TestUpdatePool(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPoolJSON, err := json.Marshal(mockPool)
	assert.NoError(t, err)
	jsonResponse, err := json.Marshal(mockPoolResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "PATCH", req.Method)
			expectedURL := fmt.Sprintf("https://test-airflow-url/api/v1/pools/%s", mockPool.Name)
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, mockPoolJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		assert.NoError(t, err)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
	})

	t.Run("error - failed to marshal pool to JSON", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		// Pass a nil pool to force JSON marshal error
		err := airflowClient.UpdatePool("test-airflow-url", Pool{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response from API")
	})

	t.Run("error - failed to execute HTTP request", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (400): Bad Request")
	})
}

func TestGetPools(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPoolResponseJSON, err := json.Marshal(mockPoolResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			assert.Equal(t, "GET", req.Method)
			expectedURL := "https://test-airflow-url/api/v1/pools"
			assert.Equal(t, expectedURL, req.URL.String())
			assert.Equal(t, "token", req.Header.Get("authorization"))

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(mockPoolResponseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		assert.NoError(t, err)
		assert.Equal(t, *mockPoolResponse, response)
	})

	t.Run("error - http request failed", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (500): Internal Service Error")
		assert.Equal(t, Response{}, response)
	})

	t.Run("error - failed to execute HTTP request", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error (400): Bad Request")
		assert.Equal(t, Response{}, response)
	})
}
