package airflowversions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *httputil.HTTPClient
}

// NewClient returns a new Client with the logger and HTTP client setup.
func NewClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

// Request represents empty request
type Request struct{}

// DoWithClient (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) DoWithClient(api *Client) (*Response, error) {
	doOpts := httputil.DoOptions{
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.Do(doOpts)
}

// Do executes the given HTTP request and returns the HTTP Response
func (r *Request) Do() (*Response, error) {
	return r.DoWithClient(NewClient(httputil.NewHTTPClient()))
}

// Do executes a query against the updates astronomer API, logging out any errors contained in the response object
func (c *Client) Do(doOpts httputil.DoOptions) (*Response, error) {
	var response httputil.HTTPResponse
	url := config.CFG.AirflowReleasesURL.GetString()
	httpResponse, err := c.HTTPClient.Do("GET", url, &doOpts)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}

	response = httputil.HTTPResponse{
		Raw:  httpResponse,
		Body: string(body),
	}
	decode := Response{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		return nil, fmt.Errorf("failed to JSON decode %s response: %w", url, err)
	}

	return &decode, nil
}
