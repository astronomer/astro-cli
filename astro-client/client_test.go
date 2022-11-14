package astro

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewgqlClient(t *testing.T) {
	client := NewGQLClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro client")
}
