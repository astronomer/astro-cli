package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArgNotSetError(t *testing.T) {
	errorMessage := ArgNotSetError("sample_argument")
	expectedErrorMesssage := "argument not set:sample_argument"
	assert.EqualError(t, errorMessage, expectedErrorMesssage)
}
