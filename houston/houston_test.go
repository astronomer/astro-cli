package houston

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewHoustonClient(t *testing.T) {
	client := NewHoustonClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new houston client")
}
