package astrov1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// APIClient is the v1 API client interface.
type APIClient = ClientWithResponsesInterface

// errRequest is returned when a v1 API call fails without a decodable error body.
var errRequest = errors.New("failed to perform request")

type apiError struct {
	Message string `json:"message"`
}

// NormalizeAPIError converts a non-success v1 API response into an error,
// decoding the API's error message when one is present.
//
// It is defined here (rather than re-exported from pkg/httputil) so this package
// can stand alone as its own Go module with no dependency on the astro-cli root
// module. The constructor that wires in context/httputil lives in the root
// module's cloud/apiclient package.
func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusNoContent {
		var decode apiError
		if err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode); err != nil {
			return fmt.Errorf("%w, status %d", errRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message)
	}
	return nil
}
