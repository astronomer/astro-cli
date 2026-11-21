package astrov1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

func TestNewV1Alpha1StreamClient(t *testing.T) {
	client := NewV1Alpha1StreamClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro v1alpha1 stream client")
}
