package astrocore

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var (
	ErrorRequest  = errors.New("failed to perform request")
	ErrorBaseURL  = errors.New("invalid baseurl")
	HTTPStatus200 = 200
)

// a shorter alias
type CoreClient = ClientWithResponsesInterface

func requestEditor(ctx httpContext.Context, req *http.Request) error {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil
	}
	baseURL := currentCtx.GetPublicRESTAPIURL()
	requestURL, err := url.Parse(baseURL + req.URL.String())
	if err != nil {
		return fmt.Errorf("%w, %s", ErrorBaseURL, baseURL)
	}
	req.URL = requestURL
	req.Header.Add("authorization", currentCtx.Token)
	return nil
}

// create api client for astro core services
func NewCoreClient(c *httputil.HTTPClient) *ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(requestEditor))
	return cl
}

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != HTTPStatus200 {
		decode := Error{}
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", ErrorRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message) //nolint:goerr113
	}
	return nil
}
