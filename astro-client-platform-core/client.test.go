package astroplatformcore

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewPlatformCoreClient(t *testing.T) {
	client := NewPlatformCoreClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro Platform Core client")
}
