package astro

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewAstroClient(t *testing.T) {
	client := NewAstroClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro client")
}
