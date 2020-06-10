package version

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIsValidVersionInValid(t *testing.T) {
	assert.False(t, isValidVersion(""))
}

func TestIsValidVersionValid(t *testing.T) {
	assert.True(t, isValidVersion("0.0.1"))
}
