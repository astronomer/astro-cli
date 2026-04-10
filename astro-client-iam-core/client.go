package astroiamcore

import (
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NormalizeAPIError is a deliberate re-export of httputil.NormalizeAPIError, allowing
// callers to override error normalization per-client without importing pkg/httputil.
var NormalizeAPIError = httputil.NormalizeAPIError

// a shorter alias
type CoreClient = ClientWithResponsesInterface

func NewIamCoreClient(c *httputil.HTTPClient, holder *credentials.CurrentCredentials) *ClientWithResponses {
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return holder.Get(), ctx.GetPublicRESTAPIURL("iam/v1beta1"), nil
	})))
	return cl
}
