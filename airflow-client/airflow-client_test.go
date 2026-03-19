package airflowclient

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

// errorRoundTripFunc is a transport that always returns a transport-level error.
type errorRoundTripFunc func(*http.Request) error

func (f errorRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, f(req)
}

func TestAirflowClient(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupSuite() {
	// Use minimal backoff so retry tests don't sleep.
	original := retryBackoff
	retryBackoff = time.Millisecond
	s.T().Cleanup(func() { retryBackoff = original })
}

func (s *Suite) TestNew() {
	client := NewAirflowClient(httputil.NewHTTPClient())
	s.NotNil(client, "Can't create new Astro client")
}

func (s *Suite) TestDoAirflowClient() {
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
	s.NoError(err)
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

func (s *Suite) TestGetConnections() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	jsonResponse, err := json.Marshal(mockConnResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("GET", req.Method)
			s.Equal("https://test-airflow-url/connections?limit=100&offset=0", req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetConnections("test-airflow-url")
		s.NoError(err)
		s.Equal(response, *mockConnResponse)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetConnections("test-airflow-url")
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})
	s.Run("error - failed to decode response", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("Invalid JSON Response")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetConnections("test-airflow-url")
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})
}

func (s *Suite) TestUpdateConnection() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	connJSON, err := json.Marshal(mockConn)
	s.NoError(err)
	jsonResponse, err := json.Marshal(mockConnResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("PATCH", req.Method)
			expectedURL := fmt.Sprintf("https://test-airflow-url/connections/%s", mockConn.ConnID)
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			s.Equal(connJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal connection to JSON", func() {
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
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})

	s.Run("error - failed to execute HTTP request", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateConnection("test-airflow-url", mockConn)
		s.Error(err)
		s.Contains(err.Error(), "API error (400): Bad Request")
	})
}

func (s *Suite) TestCreateConnection() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	connJSON, err := json.Marshal(mockConn)
	s.NoError(err)
	jsonResponse, err := json.Marshal(mockConnResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("POST", req.Method)
			expectedURL := "https://test-airflow-url/connections"
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			s.Equal(connJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal connection to JSON", func() {
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
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})

	s.Run("error - failed to execute HTTP request", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateConnection("test-airflow-url", mockConn)
		s.Error(err)
		s.Contains(err.Error(), "API error (400): Bad Request")
	})
}

func (s *Suite) TestCreateVariable() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedURL := "https://test-airflow-url/variables"

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("POST", req.Method)
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			expectedReqBody, err := json.Marshal(mockVar)
			s.NoError(err)
			s.Equal(expectedReqBody, reqBody)

			responseJSON, err := json.Marshal(mockVarResponse)
			s.NoError(err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", *mockVar)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", *mockVar)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal variable to JSON", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreateVariable("test-airflow-url", Variable{Key: "", Value: "test-value"})
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})
}

func (s *Suite) TestGetVariables() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("GET", req.Method)
			s.Equal("https://test-airflow-url/variables?limit=100&offset=0", req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			responseJSON, err := json.Marshal(mockVarResponse)
			s.NoError(err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetVariables("test-airflow-url")
		s.NoError(err)
		s.Equal(mockVarResponse.Variables, response.Variables)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		_, err := airflowClient.GetVariables("test-airflow-url")
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})
}

func (s *Suite) TestUpdateVariable() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedURL := "https://test-airflow-url/variables/test-key"

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("PATCH", req.Method)
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			expectedReqBody, err := json.Marshal(mockVar)
			s.NoError(err)
			s.Equal(expectedReqBody, reqBody)

			responseJSON, err := json.Marshal(mockVarResponse)
			s.NoError(err)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", *mockVar)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", *mockVar)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal variable to JSON", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("OK")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdateVariable("test-airflow-url", Variable{Key: "", Value: "test-value"})
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
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

func (s *Suite) TestCreatePool() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	poolJSON, err := json.Marshal(mockPool)
	s.NoError(err)
	jsonResponse, err := json.Marshal(mockPoolResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("POST", req.Method)
			expectedURL := "https://test-airflow-url/pools"
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			s.Equal(poolJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal pool to JSON", func() {
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
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})

	s.Run("error - failed to execute HTTP request", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.CreatePool("test-airflow-url", *mockPool)
		s.Error(err)
		s.Contains(err.Error(), "API error (400): Bad Request")
	})
}

func (s *Suite) TestUpdatePool() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPoolJSON, err := json.Marshal(mockPool)
	s.NoError(err)
	jsonResponse, err := json.Marshal(mockPoolResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("PATCH", req.Method)
			expectedURL := fmt.Sprintf("https://test-airflow-url/pools/%s", mockPool.Name)
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			// Check request body
			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			s.Equal(mockPoolJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		s.NoError(err)
	})

	s.Run("success - default pool", func() {
		defaultPool := Pool{
			Description:     "Default pool",
			Name:            "default_pool",
			Slots:           64,
			IncludeDeferred: true,
		}
		defaultPoolJSON, err := json.Marshal(defaultPool)
		s.NoError(err)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("PATCH", req.Method)
			expectedURL := "https://test-airflow-url/pools/default_pool?update_mask=slots&update_mask=include_deferred"
			s.Equal(expectedURL, req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			reqBody, err := io.ReadAll(req.Body)
			s.NoError(err)
			s.Equal(defaultPoolJSON, reqBody)

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("{}")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err = airflowClient.UpdatePool("test-airflow-url", defaultPool)
		s.NoError(err)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
	})

	s.Run("error - failed to marshal pool to JSON", func() {
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
		s.Error(err)
		s.Contains(err.Error(), "failed to decode response from API")
	})

	s.Run("error - failed to execute HTTP request", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		err := airflowClient.UpdatePool("test-airflow-url", *mockPool)
		s.Error(err)
		s.Contains(err.Error(), "API error (400): Bad Request")
	})
}

func (s *Suite) TestGetPools() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPoolResponseJSON, err := json.Marshal(mockPoolResponse)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("GET", req.Method)
			s.Equal("https://test-airflow-url/pools?limit=100&offset=0", req.URL.String())
			s.Equal("token", req.Header.Get("authorization"))

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(mockPoolResponseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		s.NoError(err)
		s.Equal(*mockPoolResponse, response)
	})

	s.Run("error - http request failed", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Service Error")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		s.Error(err)
		s.Contains(err.Error(), "API error (500): Internal Service Error")
		s.Equal(Response{}, response)
	})

	s.Run("error - failed to execute HTTP request", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		s.Error(err)
		s.Contains(err.Error(), "API error (400): Bad Request")
		s.Equal(Response{}, response)
	})
}

func (s *Suite) TestDoAirflowClientRetry() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("succeeds after 503s then 200", func() {
		callCount := 0
		jsonResponse, err := json.Marshal(mockConnResponse)
		s.NoError(err)

		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			if callCount < 3 {
				return &http.Response{
					StatusCode: 503,
					Body:       io.NopCloser(bytes.NewBufferString("Service Unavailable")),
					Header:     make(http.Header),
				}
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		doOpts := &httputil.DoOptions{
			Path:   "/test",
			Method: http.MethodGet,
		}
		resp, err := airflowClient.DoAirflowClient(doOpts)
		s.NoError(err)
		s.Equal(mockConnResponse.Connections, resp.Connections)
		s.Equal(3, callCount)
	})

	s.Run("exhausts retries and returns last error", func() {
		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			return &http.Response{
				StatusCode: 503,
				Body:       io.NopCloser(bytes.NewBufferString("Service Unavailable")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		doOpts := &httputil.DoOptions{
			Path:   "/test",
			Method: http.MethodGet,
		}
		_, err := airflowClient.DoAirflowClient(doOpts)
		s.Error(err)
		s.Contains(err.Error(), "API error (503)")
		// 1 initial attempt + maxRetries retries
		s.Equal(maxRetries+1, callCount)
	})

	s.Run("does not retry 4xx errors", func() {
		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		doOpts := &httputil.DoOptions{
			Path:   "/test",
			Method: http.MethodGet,
		}
		_, err := airflowClient.DoAirflowClient(doOpts)
		s.Error(err)
		s.Contains(err.Error(), "API error (400)")
		s.Equal(1, callCount)
	})

	s.Run("does not retry non-GET requests", func() {
		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			return &http.Response{
				StatusCode: 503,
				Body:       io.NopCloser(bytes.NewBufferString("Service Unavailable")),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		doOpts := &httputil.DoOptions{
			Path:   "/test",
			Method: http.MethodPost,
		}
		_, err := airflowClient.DoAirflowClient(doOpts)
		s.Error(err)
		s.Contains(err.Error(), "API error (503)")
		s.Equal(1, callCount)
	})

	s.Run("does not retry on context cancellation", func() {
		callCount := 0
		cancelTransport := httputil.NewHTTPClient()
		cancelTransport.HTTPClient.Transport = errorRoundTripFunc(func(*http.Request) error {
			callCount++
			return stdctx.Canceled
		})
		airflowClient := NewAirflowClient(cancelTransport)

		doOpts := &httputil.DoOptions{
			Path:   "/test",
			Method: http.MethodGet,
		}
		_, err := airflowClient.DoAirflowClient(doOpts)
		s.ErrorIs(err, stdctx.Canceled)
		s.Equal(1, callCount)
	})
}

func (s *Suite) TestGetConnectionsPagination() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("fetches all pages", func() {
		// Page 1: 100 connections, TotalEntries: 150
		page1Conns := make([]Connection, 100)
		for i := range page1Conns {
			page1Conns[i] = Connection{ConnID: fmt.Sprintf("conn-%d", i)}
		}
		page1 := Response{Connections: page1Conns, TotalEntries: 150}

		// Page 2: 50 connections (last page)
		page2Conns := make([]Connection, 50)
		for i := range page2Conns {
			page2Conns[i] = Connection{ConnID: fmt.Sprintf("conn-%d", 100+i)}
		}
		page2 := Response{Connections: page2Conns, TotalEntries: 150}

		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			var resp Response
			if callCount == 1 {
				s.Equal("https://test-airflow-url/connections?limit=100&offset=0", req.URL.String())
				resp = page1
			} else {
				s.Equal("https://test-airflow-url/connections?limit=100&offset=100", req.URL.String())
				resp = page2
			}
			responseJSON, err := json.Marshal(resp)
			s.NoError(err)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetConnections("test-airflow-url")
		s.NoError(err)
		s.Len(response.Connections, 150)
		s.Equal(2, callCount)
	})
}

func (s *Suite) TestGetVariablesPagination() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("fetches all pages", func() {
		page1Vars := make([]Variable, 100)
		for i := range page1Vars {
			page1Vars[i] = Variable{Key: fmt.Sprintf("var-%d", i), Value: "v"}
		}
		page1 := Response{Variables: page1Vars, TotalEntries: 120}

		page2Vars := make([]Variable, 20)
		for i := range page2Vars {
			page2Vars[i] = Variable{Key: fmt.Sprintf("var-%d", 100+i), Value: "v"}
		}
		page2 := Response{Variables: page2Vars, TotalEntries: 120}

		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			var resp Response
			if callCount == 1 {
				s.Equal("https://test-airflow-url/variables?limit=100&offset=0", req.URL.String())
				resp = page1
			} else {
				s.Equal("https://test-airflow-url/variables?limit=100&offset=100", req.URL.String())
				resp = page2
			}
			responseJSON, err := json.Marshal(resp)
			s.NoError(err)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetVariables("test-airflow-url")
		s.NoError(err)
		s.Len(response.Variables, 120)
		s.Equal(2, callCount)
	})
}

func (s *Suite) TestGetPoolsPagination() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("fetches all pages", func() {
		page1Pools := make([]Pool, 100)
		for i := range page1Pools {
			page1Pools[i] = Pool{Name: fmt.Sprintf("pool-%d", i), Slots: 5}
		}
		page1 := Response{Pools: page1Pools, TotalEntries: 110}

		page2Pools := make([]Pool, 10)
		for i := range page2Pools {
			page2Pools[i] = Pool{Name: fmt.Sprintf("pool-%d", 100+i), Slots: 5}
		}
		page2 := Response{Pools: page2Pools, TotalEntries: 110}

		callCount := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			callCount++
			var resp Response
			if callCount == 1 {
				s.Equal("https://test-airflow-url/pools?limit=100&offset=0", req.URL.String())
				resp = page1
			} else {
				s.Equal("https://test-airflow-url/pools?limit=100&offset=100", req.URL.String())
				resp = page2
			}
			responseJSON, err := json.Marshal(resp)
			s.NoError(err)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(responseJSON)),
				Header:     make(http.Header),
			}
		})
		airflowClient := NewAirflowClient(client)

		response, err := airflowClient.GetPools("test-airflow-url")
		s.NoError(err)
		s.Len(response.Pools, 110)
		s.Equal(2, callCount)
	})
}
