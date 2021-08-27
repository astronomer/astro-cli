package houston

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/logger"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/pkg/errors"
)

var PermissionsError = errors.New("You do not have the appropriate permissions for that")
var PermissionsErrorVerbose = errors.New("You do not have the appropriate permissions for that: Your token has expired. Please log in again.")
var log = logger.NewLogger()

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *httputil.HTTPClient
}

// NewHoustonClient returns a new Client with the logger and HTTP client setup.
func NewHoustonClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

// GraphQLQuery wraps a graphql query string
type GraphQLQuery struct {
	Query string `json:"query"`
}

type Request struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// Do (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) DoWithClient(api *Client) (*Response, error) {
	doOpts := httputil.DoOptions{
		Data: r,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.Do(doOpts)
}

// Do (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) Do() (*Response, error) {
	return r.DoWithClient(NewHoustonClient(httputil.NewHTTPClient()))
}

// Do executes a query against the Houston API, logging out any errors contained in the response object
func (c *Client) Do(doOpts httputil.DoOptions) (*Response, error) {
	cl, err := cluster.GetCurrentCluster()
	if err != nil {
		return nil, err
	}

	// set headers
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}
	log.Debugf("This is the url %s \n", cl.GetAPIURL())
	log.Debugf("Request Data: %v\n", doOpts.Data)
	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", cl.GetAPIURL(), &doOpts)
	if err != nil {
		log.Debugf("HTTP request ERROR: %s", err.Error())
		return nil, err
	}
	defer httpResponse.Body.Close()

	// strings.NewReader(jsonStream)
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
		return nil, errors.Wrap(err, "Failed to JSON decode Houston response")
	}
	log.Debugf("Response Data: %v\n", string(body))
	// Houston Specific Errors
	if decode.Errors != nil {
		err = errors.New(decode.Errors[0].Message)
		if err.Error() == PermissionsError.Error() {
			return nil, PermissionsErrorVerbose
		}
		return nil, err
	}

	return &decode, nil
}
