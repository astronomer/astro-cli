package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// NormalizeAPIError keeps the text it always had and adds the status, which a
// JSON body's message would otherwise hide.
func TestNormalizeAPIErrorCarriesTheStatus(t *testing.T) {
	jsonErr := NormalizeAPIError(&http.Response{StatusCode: http.StatusUnauthorized}, []byte(`{"message":"token expired"}`))
	assert.EqualError(t, jsonErr, "token expired")
	assert.True(t, HasStatus(jsonErr, http.StatusUnauthorized))
	assert.False(t, HasStatus(jsonErr, http.StatusForbidden))
	assert.False(t, errors.Is(jsonErr, ErrorRequest))

	bare := NormalizeAPIError(&http.Response{StatusCode: http.StatusBadGateway}, []byte("<html>"))
	assert.EqualError(t, bare, "failed to perform request, status 502")
	assert.True(t, errors.Is(bare, ErrorRequest))
	assert.True(t, HasStatus(fmt.Errorf("listing: %w", bare), http.StatusBadGateway), "a wrapped error lost its status")

	assert.NoError(t, NormalizeAPIError(&http.Response{StatusCode: http.StatusOK}, nil))
	assert.False(t, HasStatus(errors.New("status 401"), http.StatusUnauthorized), "text alone is not a status")
}
