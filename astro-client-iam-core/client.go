package astroiamcore

import (
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NormalizeAPIError is a deliberate re-export of httputil.NormalizeAPIError, allowing
// callers to override error normalization per-client without importing pkg/httputil.
var NormalizeAPIError = httputil.NormalizeAPIError

// a shorter alias
type CoreClient = ClientWithResponsesInterface

func NewIamCoreClient(c *httputil.HTTPClient) *ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("iam/v1beta1"), nil
	})))
	return cl
}
