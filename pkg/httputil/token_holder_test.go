package httputil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

func TestTokenHolder(t *testing.T) {
	h := &httputil.TokenHolder{}
	assert.Equal(t, "", h.Get())

	h.Set("Bearer abc")
	assert.Equal(t, "Bearer abc", h.Get())

	h.Set("")
	assert.Equal(t, "", h.Get())
}
