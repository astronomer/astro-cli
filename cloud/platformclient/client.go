package platformclient

import (
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NewPlatformCoreClient creates an API client for Astro platform core services.
func NewPlatformCoreClient(c *httputil.HTTPClient, holder *credentials.CurrentCredentials) *astroplatformcore.ClientWithResponses {
	return astroplatformcore.NewPlatformCoreClient(c, holder)
}
