package astroiamcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var (
	ErrorRequest  = errors.New("failed to perform request")
	ErrorBaseURL  = errors.New("invalid baseurl")
	HTTPStatus200 = 200
	HTTPStatus204 = 204
)

// a shorter alias
type CoreClient = ClientWithResponsesInterface

// create api client for astro iam core services
func NewIamCoreClient(c *httputil.HTTPClient) *ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(astrocore.CoreRequestEditor))
	return cl
}

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != HTTPStatus200 && httpResp.StatusCode != HTTPStatus204 {
		decode := Error{}
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", ErrorRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message) //nolint:goerr113
	}
	return nil
}
