package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPypiVersionInvalidHostFailure(t *testing.T) {
	_, err := GetPypiVersion("http://abcd")
	expectedErrMessage := "error getting latest release version for project url http://abcd,  Get \"http://abcd\": dial tcp: lookup abcd: no such host"
	assert.EqualError(t, err, expectedErrMessage)
}
