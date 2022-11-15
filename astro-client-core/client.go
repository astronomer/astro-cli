package astrocore

import (
	"bytes"
	http_context "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var (
	RequestError = errors.New("failed to perform request")
)

// a shorter alias
type CoreClient = ClientWithResponsesInterface

func requestEditor(httpContext http_context.Context, req *http.Request) error {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil
	}
	req.Header.Add("authorization", currentCtx.Token)
	return nil
}

// create api client for astro core services
func NewCoreClient(c *httputil.HTTPClient) (*ClientWithResponses, error) {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	cl, err := NewClientWithResponses(currentCtx.GetPublicRESTAPIURL(), WithHTTPClient(c.HTTPClient), WithRequestEditorFn(requestEditor))
	if err != nil {
		return nil, err
	}
	return cl, nil
}

const HttpStatus200 = 200

func NormalizeAPIError(httpResp *http.Response, body []byte, err error) error {
	if err != nil {
		return err
	}
	if httpResp.StatusCode != HttpStatus200 {
		decode := Error{}
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", RequestError, httpResp.StatusCode)
		}
		return errors.New(decode.Message)
	}
	return nil
}
