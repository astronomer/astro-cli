package httputil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTransportError(t *testing.T) {
	dial := func(err error) error {
		return &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", err)}}
	}

	// Transient: a retry can fix them.
	for name, err := range map[string]error{
		"connection refused":     dial(syscall.ECONNREFUSED),
		"connection reset":       dial(syscall.ECONNRESET),
		"host unreachable":       dial(syscall.EHOSTUNREACH),
		"network unreachable":    dial(syscall.ENETUNREACH),
		"wrapped":                fmt.Errorf("listing organizations: %w", dial(syscall.ECONNREFUSED)),
		"dns":                    &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.DNSError{Err: "no such host", Name: "api.invalid", IsNotFound: true}},
		"dns timeout":            &net.DNSError{Err: "i/o timeout", Name: "api.invalid", IsTimeout: true},
		"windows-style refusal":  &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connectex", syscall.Errno(10061))}},
		"winsock refusal":        &url.Error{Op: "Get", URL: "http://proxy.invalid/", Err: os.NewSyscallError("connectex", syscall.Errno(10061))},
		"winsock reset":          &url.Error{Op: "Get", URL: "http://proxy.invalid/", Err: &net.OpError{Op: "proxyconnect", Err: os.NewSyscallError("wsarecv", syscall.Errno(10054))}},
		"winsock call only":      &url.Error{Op: "Get", URL: "http://proxy.invalid/", Err: &net.OpError{Op: "proxyconnect", Err: os.NewSyscallError("wsarecv", errors.New("unmapped"))}},
		"server busy":            fmt.Errorf("refresh: %w", NewTransientStatusError(http.StatusServiceUnavailable)),
		"connection ended early": &url.Error{Op: "Get", URL: "http://api.invalid/", Err: io.ErrUnexpectedEOF},
		"eof":                    &url.Error{Op: "Get", URL: "http://api.invalid/", Err: io.EOF},
		"deadline":               context.DeadlineExceeded,
		"timeout":                &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "read", Err: timeoutErr{}}},
	} {
		assert.True(t, IsTransportError(err), name)
	}

	// Not transient, or not a transport failure at all.
	for name, err := range map[string]error{
		"nil":                       nil,
		"a body that decoded empty": fmt.Errorf("decoding: %w", io.EOF),
		"a body cut short":          fmt.Errorf("decoding: %w", io.ErrUnexpectedEOF),
		"a tls alert":               &url.Error{Op: "Get", URL: "https://api.invalid/", Err: &net.OpError{Op: "remote error", Err: errors.New("tls: bad certificate")}},
		"canceled":                  &url.Error{Op: "Get", URL: "http://api.invalid/", Err: context.Canceled},
		"unknown authority":         &url.Error{Op: "Get", URL: "https://api.invalid/", Err: x509.UnknownAuthorityError{}},
		"invalid certificate":       &url.Error{Op: "Get", URL: "https://api.invalid/", Err: x509.CertificateInvalidError{Reason: x509.Expired}},
		"hostname mismatch":         &url.Error{Op: "Get", URL: "https://api.invalid/", Err: x509.HostnameError{Host: "api.invalid"}},
		"certificate verification":  &url.Error{Op: "Get", URL: "https://api.invalid/", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
		"not tls":                   &url.Error{Op: "Get", URL: "https://api.invalid/", Err: tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}},
		"unsupported scheme":        &url.Error{Op: "Get", URL: "ftp://api.invalid/", Err: errors.New(`unsupported protocol scheme "ftp"`)},
		"malformed url":             &url.Error{Op: "parse", URL: "http://a b/", Err: url.InvalidHostError(" ")},
		"a status":                  NormalizeAPIError(&http.Response{StatusCode: http.StatusServiceUnavailable}, nil),
		"boom":                      errors.New("boom"),
	} {
		assert.False(t, IsTransportError(err), name)
	}
}

// timeoutErr is a net.Error that timed out.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
