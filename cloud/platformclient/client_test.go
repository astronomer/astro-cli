package platformclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

func TestNewPlatformCoreClient(t *testing.T) {
	client := NewPlatformCoreClient(httputil.NewHTTPClient(), &credentials.CurrentCredentials{})
	assert.NotNil(t, client, "Can't create new Astro Platform Core client")
}
