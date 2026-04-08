package astroiamcore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

func TestNewIamCoreClient(t *testing.T) {
	client := NewIamCoreClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro IAM Core client")
}
