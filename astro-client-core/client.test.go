package astrocore

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewCoreClient(t *testing.T) {
	client := NewCoreClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro Core client")
}
