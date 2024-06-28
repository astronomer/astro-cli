package airflowclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var errDecode = errors.New("failed to decode response from API")

type Client interface {
	// connections
	GetConnections(airflowURL string) (Response, error)
	CreateConnection(airflowURL string, conn *Connection) error
	UpdateConnection(airflowURL string, conn *Connection) error
	// variables
	GetVariables(airflowURL string) (Response, error)
	CreateVariable(airflowURL string, variable Variable) error
	UpdateVariable(airflowURL string, variable Variable) error
	// pools
	GetPools(airflowURL string) (Response, error)
	CreatePool(airflowURL string, pool Pool) error
	UpdatePool(airflowURL string, pool Pool) error
}

// Client containers the logger and HTTPClient used to communicate with the Astronomer API
type HTTPClient struct {
	*httputil.HTTPClient
}

// NewAstroClient returns a new Client with the logger and HTTP client setup.
func NewAirflowClient(c *httputil.HTTPClient) *HTTPClient {
	return &HTTPClient{
		c,
	}
}

func (c *HTTPClient) GetConnections(airflowURL string) (Response, error) {
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/connections",
		Method: http.MethodGet,
	}

	response, err := c.DoAirflowClient(doOpts)
	if err != nil {
		return Response{}, err
	}

	return *response, nil
}

func (c *HTTPClient) CreateConnection(airflowURL string, conn *Connection) error {
	// Convert the connection struct to JSON bytes
	connJSON, err := json.Marshal(&conn)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/connections",
		Method: http.MethodPost,
		Data:   connJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}
	return nil
}

func (c *HTTPClient) UpdateConnection(airflowURL string, conn *Connection) error {
	// Convert the connection struct to JSON bytes
	connJSON, err := json.Marshal(&conn)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   fmt.Sprintf("https://%s/api/v1/connections/%s", airflowURL, conn.ConnID),
		Method: http.MethodPatch,
		Data:   connJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) GetVariables(airflowURL string) (Response, error) {
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/variables",
		Method: http.MethodGet,
	}

	response, err := c.DoAirflowClient(doOpts)
	if err != nil {
		return Response{}, err
	}

	return *response, nil
}

func (c *HTTPClient) CreateVariable(airflowURL string, variable Variable) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(variable)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/variables",
		Method: http.MethodPost,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}
	return nil
}

func (c *HTTPClient) UpdateVariable(airflowURL string, variable Variable) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(variable)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   fmt.Sprintf("https://%s/api/v1/variables/%s", airflowURL, variable.Key),
		Method: http.MethodPatch,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) GetPools(airflowURL string) (Response, error) {
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/pools",
		Method: http.MethodGet,
	}

	response, err := c.DoAirflowClient(doOpts)
	if err != nil {
		return Response{}, err
	}

	return *response, nil
}

func (c *HTTPClient) CreatePool(airflowURL string, pool Pool) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/api/v1/pools",
		Method: http.MethodPost,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}
	return nil
}

func (c *HTTPClient) UpdatePool(airflowURL string, pool Pool) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   fmt.Sprintf("https://%s/api/v1/pools/%s", airflowURL, pool.Name),
		Method: http.MethodPatch,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) DoAirflowClient(doOpts *httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext() // get current context
	if err != nil {
		return nil, err
	}

	if cl.Token != "" {
		doOpts.Headers = map[string]string{
			"authorization": cl.Token,
		}
	}

	response, err := c.Do(doOpts)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// Check the response status code
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	decode := Response{}
	err = json.NewDecoder(response.Body).Decode(&decode)
	if err != nil {
		return nil, errDecode
	}

	return &decode, nil
}
