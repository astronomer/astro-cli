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
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestAirflowClient(t *testing.T) {
	suite.Run(t, new(Suite))
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
			s.Equal("https://test-airflow-url/api/v1/connections", req.URL.String())
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
			expectedURL := fmt.Sprintf("https://test-airflow-url/api/v1/connections/%s", mockConn.ConnID)
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
			expectedURL := "https://test-airflow-url/api/v1/connections"
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
	expectedURL := "https://test-airflow-url/api/v1/variables"

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
	expectedURL := "https://test-airflow-url/api/v1/variables"

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			s.Equal("GET", req.Method)
			s.Equal(expectedURL, req.URL.String())
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
		s.Equal(mockVarResponse, &response)
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
	expectedURL := "https://test-airflow-url/api/v1/variables/test-key"

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
			expectedURL := "https://test-airflow-url/api/v1/pools"
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
			expectedURL := fmt.Sprintf("https://test-airflow-url/api/v1/pools/%s", mockPool.Name)
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
			expectedURL := "https://test-airflow-url/api/v1/pools"
			s.Equal(expectedURL, req.URL.String())
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
