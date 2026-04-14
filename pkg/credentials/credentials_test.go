package credentials_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/credentials"
)

func TestCurrentCredentials(t *testing.T) {
	h := &credentials.CurrentCredentials{}
	assert.Equal(t, "", h.Get())

	h.Set("Bearer abc")
	assert.Equal(t, "Bearer abc", h.Get())

	h.Set("")
	assert.Equal(t, "", h.Get())
}
