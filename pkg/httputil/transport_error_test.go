package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTransportError(t *testing.T) {
	refused := &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	assert.True(t, IsTransportError(refused))
	assert.True(t, IsTransportError(fmt.Errorf("listing organizations: %w", refused)))
	assert.True(t, IsTransportError(&net.DNSError{Err: "no such host", Name: "api.invalid"}))
	assert.True(t, IsTransportError(fmt.Errorf("read: %w", io.ErrUnexpectedEOF)))
	assert.True(t, IsTransportError(context.DeadlineExceeded))

	// An answer, whatever it says, is not a failure to get one.
	assert.False(t, IsTransportError(NormalizeAPIError(&http.Response{StatusCode: http.StatusServiceUnavailable}, nil)))
	assert.False(t, IsTransportError(&url.Error{Op: "Get", URL: "http://api.invalid/", Err: context.Canceled}), "an interrupted request is the run ending")
	assert.False(t, IsTransportError(errors.New("boom")))
	assert.False(t, IsTransportError(nil))
}
