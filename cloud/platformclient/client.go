package platformclient

import (
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NewPlatformCoreClient creates an API client for Astro platform core services.
func NewPlatformCoreClient(c *httputil.HTTPClient, holder *httputil.TokenHolder) *astroplatformcore.ClientWithResponses {
	return astroplatformcore.NewPlatformCoreClient(c, holder)
}
