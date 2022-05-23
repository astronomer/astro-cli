package astro

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"
)

const (
	AstronomerConnectionErrMsg = "Cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions.\n\nDetails"

	permissionsErrMsg = "You do not have the appropriate permissions for that"
)

// Client containers the logger and HTTPClient used to communicate with the Astronomer API
type HTTPClient struct {
	*httputil.HTTPClient
}

// NewAstroClient returns a new Client with the logger and HTTP client setup.
func NewAstroClient(c *httputil.HTTPClient) *HTTPClient {
	return &HTTPClient{
		c,
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
func (r *Request) DoWithClient(api *HTTPClient) (*Response, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	doOpts := httputil.DoOptions{
		Data: data,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.Do(doOpts)
}

// Do (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) Do() (*Response, error) {
	return r.DoWithClient(NewAstroClient(httputil.NewHTTPClient()))
}

// Do executes a query against the Astronomer API, logging out any errors contained in the response object
func (c *HTTPClient) Do(doOpts httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	// set headers
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}
	doOpts.Headers["apollographql-client-name"] = "cli"
	doOpts.Headers["apollographql-client-version"] = version.CurrVersion

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", cl.GetCloudAPIURL(), &doOpts)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
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
		return nil, fmt.Errorf("Failed to JSON decode Astronomer API response: %w", err) //nolint
	}

	// Astrohub Specific Errors
	if decode.Errors != nil {
		if decode.Errors[0].Message == permissionsErrMsg {
			return nil, fmt.Errorf("Your token has expired. Please log in again: %s", decode.Errors[0].Message) //nolint
		}
		return nil, fmt.Errorf(decode.Errors[0].Message) //nolint
	}

	return &decode, nil
}

// Do (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) DoWithPublicClient(api *HTTPClient) (*Response, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	doOpts := httputil.DoOptions{
		Data: data,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.DoPublic(doOpts)
}

// Do (request) is a wrapper to more easily pass variables to a client.Do request
func (r *Request) DoPublic() (*Response, error) {
	return r.DoWithPublicClient(NewAstroClient(httputil.NewHTTPClient()))
}

// Do executes a query against the Astrohub API, logging out any errors contained in the response object
func (c *HTTPClient) DoPublic(doOpts httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	// set headers
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}
	doOpts.Headers["apollographql-client-name"] = "cli"
	doOpts.Headers["apollographql-client-version"] = version.CurrVersion

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", cl.GetPublicAPIURL(), &doOpts)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
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
		return nil, fmt.Errorf("Failed to JSON decode Astronomer API response: %w", err) //nolint
	}

	// Astronomer API Specific Errors
	if decode.Errors != nil {
		if decode.Errors[0].Message == permissionsErrMsg {
			return nil, fmt.Errorf("Your token has expired. Please log in again: %w", err) //nolint
		}
		return nil, err
	}

	return &decode, nil
}
