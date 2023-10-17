package astro

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"
)

const (
	AstronomerConnectionErrMsg = "cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions. If you are using an API Key or Token make sure your context is correct.\n\nDetails"

	permissionsErrMsg = "you do not have the appropriate permissions for that"
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
func (r *Request) DoWithPublicClient(api *HTTPClient) (*Response, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	doOpts := &httputil.DoOptions{
		Data: data,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.doPublicGraphQLQuery(doOpts)
}

// Set the path and Authorization header for a REST request to core API
func (c *HTTPClient) prepareRESTRequest(doOpts *httputil.DoOptions) error {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}
	doOpts.Path = cl.GetPublicRESTAPIURL("") + doOpts.Path
	doOpts.Headers["x-astro-client-identifier"] = "cli" //nolint: goconst
	doOpts.Headers["x-astro-client-version"] = version.CurrVersion
	return nil
}

// DoPublicRESTQuery executes a query against core API
func (c *HTTPClient) DoPublicRESTQuery(doOpts *httputil.DoOptions) (*httputil.HTTPResponse, error) {
	err := c.prepareRESTRequest(doOpts)
	if err != nil {
		return nil, err
	}
	return c.DoPublic(doOpts)
}

// DoPublicRESTQuery executes a query against core API and returns a raw buffer to stream
func (c *HTTPClient) DoPublicRESTStreamQuery(doOpts *httputil.DoOptions) (io.ReadCloser, error) {
	err := c.prepareRESTRequest(doOpts)
	if err != nil {
		return nil, err
	}
	return c.DoPublicStream(doOpts)
}

// DoPublicGraphQLQuery executes a query against Astrohub GraphQL API, logging out any errors contained in the response object
func (c *HTTPClient) doPublicGraphQLQuery(doOpts *httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}

	if cl.Organization != "" {
		doOpts.Headers["astro-current-org-id"] = cl.Organization
	}

	doOpts.Headers["apollographql-client-name"] = "cli" //nolint: goconst
	doOpts.Headers["apollographql-client-version"] = version.CurrVersion
	doOpts.Method = http.MethodPost
	doOpts.Path = cl.GetPublicGraphQLAPIURL()

	response, err := c.DoPublic(doOpts)
	if err != nil {
		return nil, fmt.Errorf("Error processing GraphQL request: %w", err)
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
		return nil, fmt.Errorf(decode.Errors[0].Message) //nolint
	}

	return &decode, nil
}

func (c *HTTPClient) DoPublic(doOpts *httputil.DoOptions) (*httputil.HTTPResponse, error) {
	httpResponse, err := c.HTTPClient.Do(doOpts)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}

	response := &httputil.HTTPResponse{
		Raw:  httpResponse,
		Body: string(body),
	}

	return response, nil
}

func (c *HTTPClient) DoPublicStream(doOpts *httputil.DoOptions) (io.ReadCloser, error) {
	httpResponse, err := c.HTTPClient.Do(doOpts)
	if err != nil {
		return nil, err
	}
	return httpResponse.Body, nil
}
