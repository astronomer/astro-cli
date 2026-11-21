package httputil

import (
	"bytes"
	"cmp"
	httpContext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/version"
)

var ErrorRequest = errors.New("request failed")

type apiError struct {
	Message string `json:"message"`
}

// StatusError is the error NormalizeAPIError returns. Its text is the API's
// message, or ErrorRequest and the status when the body carries none, and the
// status travels with it, so a caller can tell a refused login (401) from the
// message by errors.As rather than by reading the text.
type StatusError struct {
	StatusCode int
	msg        string
	wrapped    error
	// method and path are the request's, when the response carried it. A 5xx
	// to a request that changes state leaves its outcome unknown
	// (OutcomeUnknown).
	method, path string
}

// outcomeUnknown reports a 5xx answer to a request that changes state: the
// server may have acted before it failed.
func (e *StatusError) outcomeUnknown() bool {
	return e.StatusCode >= http.StatusInternalServerError && isMutation(e.method)
}

func (e *StatusError) Error() string { return e.msg }

func (e *StatusError) Unwrap() error { return e.wrapped }

// HasStatus reports that err is, or wraps, a StatusError with the given status.
func HasStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == status
}

// NormalizeAPIError returns the StatusError for a response that is not a
// success, and nil for one that is. A 5xx answer to a request that changes
// state says so in its text: the change may have been applied.
func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode == http.StatusOK || httpResp.StatusCode == http.StatusNoContent {
		return nil
	}
	se := statusError(httpResp, body)
	if se.outcomeUnknown() {
		se.msg += "; " + unknownOutcomeText(se.path)
	}
	return se
}

func statusError(httpResp *http.Response, body []byte) *StatusError {
	se := &StatusError{StatusCode: httpResp.StatusCode}
	if httpResp.Request != nil {
		se.method = httpResp.Request.Method
		if httpResp.Request.URL != nil {
			se.path = httpResp.Request.URL.Path
		}
	}
	var decode apiError
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode); err != nil {
		// No message from the API, most often a proxy's error page. The
		// status is the detail, after the fact that nothing explained it.
		se.msg = fmt.Sprintf("%s: the Astro API sent no error message (HTTP %d)", ErrorRequest, httpResp.StatusCode)
		se.wrapped = ErrorRequest
		return se
	}
	if pr, ok := unlinkedPRPreview(httpResp, decode.Message); ok {
		se.msg = fmt.Sprintf("you're logged in to PR previews, but %s has no user for you yet. Run astro login %s to create it, then run the command again", pr, pr)
		return se
	}
	se.msg = decode.Message
	return se
}

// unlinkedPRPreview reports the PR preview that refused a request because the
// user is not in its database yet. One login works for every PR preview, but
// each preview has its own database and answers 403 "oauth_user with id … is
// forbidden" until a login there creates the user.
func unlinkedPRPreview(httpResp *http.Response, message string) (string, bool) {
	if httpResp.StatusCode != http.StatusForbidden || httpResp.Request == nil ||
		!strings.HasPrefix(message, "oauth_user with id ") || !strings.HasSuffix(message, " is forbidden") {
		return "", false
	}
	return domainutil.PRPreviewOfAPIHost(httpResp.Request.URL.Hostname())
}

// NewRequestEditorFn returns a request editor that sets auth headers and the
// base URL. getTokenAndURL is called per-request and should return the bearer
// token and the resolved base URL for the given API path (e.g. from
// context.GetCurrentContext).
func NewRequestEditorFn(getTokenAndURL func() (token, baseURL string, err error)) func(httpContext.Context, *http.Request) error {
	return func(ctx httpContext.Context, req *http.Request) error {
		token, baseURL, err := getTokenAndURL()
		if err != nil {
			return nil
		}
		operatingSystem := runtime.GOOS
		arch := runtime.GOARCH
		requestURL, err := url.Parse(baseURL + req.URL.String())
		if err != nil {
			return fmt.Errorf("%w, %s", ErrorBaseURL, baseURL)
		}
		req.URL = requestURL
		req.Header.Add("authorization", token)
		// The client version is this CLI's own, except under the deploy action,
		// which ships its own and reports that instead. Its version comes from
		// the environment, so fall back to ours when it exported none: the
		// header has to name something, or the API cannot tell what called it.
		identifier, clientVersion := "cli", version.Current()
		switch {
		case os.Getenv("DEPLOY_ACTION") == "true" && os.Getenv("GITHUB_ACTIONS") == "true":
			identifier = "deploy-action"
			clientVersion = cmp.Or(os.Getenv("DEPLOY_ACTION_VERSION"), clientVersion)
		case os.Getenv("GITHUB_ACTIONS") == "true":
			identifier = "github-action"
		}
		req.Header.Add("x-astro-client-identifier", identifier)
		req.Header.Add("x-astro-client-version", clientVersion)
		req.Header.Add("x-client-os-identifier", operatingSystem+"-"+arch)
		req.Header.Add("User-Agent", fmt.Sprintf("astro-cli/%s", version.Current()))
		return nil
	}
}

// IsTransportError reports whether err is a request that failed in transit in
// a way a retry can fix: the host refused or reset the connection or could not
// be reached, its name did not resolve, the connection ended early, or the
// request timed out, or a server that answered 5xx or 429 and so said only
// that it could not take the request now (TransientStatusError). Any other HTTP
// status is an answer, and is a StatusError instead. A failure a retry will not fix is not one: a canceled
// request (the run ended), a certificate or TLS failure, a URL that is
// malformed or names an unsupported scheme.
func IsTransportError(err error) bool {
	if err == nil || errors.Is(err, httpContext.Canceled) {
		return false
	}
	for _, errno := range []error{
		syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.EPIPE,
		syscall.EHOSTUNREACH, syscall.ENETUNREACH,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}
	for _, errno := range windowsTransportErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}
	var (
		transientErr *TransientStatusError
		sysErr       *os.SyscallError
		dnsErr       *net.DNSError
		opErr        *net.OpError
		urlErr       *url.Error
		netErr       net.Error
	)
	switch {
	case errors.As(err, &transientErr):
		return true
	case errors.As(err, &sysErr) && windowsSocketCalls[sysErr.Syscall]:
		return true
	case errors.As(err, &dnsErr):
		return true
	case errors.As(err, &opErr) && (opErr.Op == "dial" || opErr.Op == "read" || opErr.Op == "write"):
		// What a connection that could not be made or broke is on every
		// platform: Windows reports a refusal as its own socket error, which
		// is none of the errno values above. A TLS alert is Op "remote error".
		return true
	case errors.Is(err, httpContext.DeadlineExceeded):
		return true
	case errors.As(err, &urlErr) && (errors.Is(urlErr.Err, io.EOF) || errors.Is(urlErr.Err, io.ErrUnexpectedEOF)):
		// The client's request ended early. The same EOF from decoding a body
		// the server did send is not one.
		return true
	case errors.As(err, &netErr):
		return netErr.Timeout()
	}
	return false
}

// The Winsock errors for a connection that was refused, reset, aborted, timed
// out or could not be routed. They are numbers, not syscall.E* names: on
// Windows those names are Go's own invented values, which no Winsock call
// returns, and the numbers are harmless elsewhere.
const (
	wsaENETDOWN     syscall.Errno = 10050
	wsaENETUNREACH  syscall.Errno = 10051
	wsaENETRESET    syscall.Errno = 10052
	wsaECONNABORTED syscall.Errno = 10053
	wsaECONNRESET   syscall.Errno = 10054
	wsaENOTCONN     syscall.Errno = 10057
	wsaESHUTDOWN    syscall.Errno = 10058
	wsaETIMEDOUT    syscall.Errno = 10060
	wsaECONNREFUSED syscall.Errno = 10061
	wsaEHOSTDOWN    syscall.Errno = 10064
	wsaEHOSTUNREACH syscall.Errno = 10065
)

// windowsTransportErrnos are the Winsock errors IsTransportError recognizes.
var windowsTransportErrnos = []error{
	wsaENETDOWN,
	wsaENETUNREACH,
	wsaENETRESET,
	wsaECONNABORTED,
	wsaECONNRESET,
	wsaENOTCONN,
	wsaESHUTDOWN,
	wsaETIMEDOUT,
	wsaECONNREFUSED,
	wsaEHOSTDOWN,
	wsaEHOSTUNREACH,
}

// windowsSocketCalls are the Windows socket calls whose failure, wrapped in an
// os.SyscallError, is a connection that failed.
var windowsSocketCalls = map[string]bool{
	"connectex": true, "wsarecv": true, "wsasend": true, "wsarecvfrom": true, "wsasendto": true,
}

// TransientStatusError is a server answer that says nothing about the request
// itself, only that the server could not take it now: 5xx, or 429. A caller
// with no body to read a verdict from (a proxy's HTML page) returns it, so
// that IsTransportError reads it as a failure a retry can fix.
type TransientStatusError struct{ StatusCode int }

// NewTransientStatusError returns the error for a 5xx or 429 answer.
func NewTransientStatusError(status int) error { return &TransientStatusError{StatusCode: status} }

func (e *TransientStatusError) Error() string {
	return fmt.Sprintf("the server answered %d %s", e.StatusCode, http.StatusText(e.StatusCode))
}
