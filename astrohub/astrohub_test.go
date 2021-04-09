package astrohub

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestNewAstrohubClient(t *testing.T) {
	client := NewAstrohubClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astrohub client")
}