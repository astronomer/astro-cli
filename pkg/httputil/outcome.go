package httputil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

// isMutation reports whether method changes state on the server. A failure of
// one after the request was sent leaves the outcome unknown: the server may
// have acted before it failed.
func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// OutcomeUnknownError is a transport failure of a request that changes state,
// after the request may have reached the server. It wraps the failure, so
// IsTransportError and errors.Is see through it.
type OutcomeUnknownError struct {
	Method string
	Path   string
	Err    error
}

func (e *OutcomeUnknownError) Error() string {
	return e.Err.Error() + "; " + unknownOutcomeText(e.Path)
}

func (e *OutcomeUnknownError) Unwrap() error { return e.Err }

// OutcomeUnknown reports that err is a failure of a request that changes state
// whose outcome is unknown: the server answered 5xx, or the connection broke
// after the request was sent. The change may have been applied, so the caller
// should look at the state before sending it again.
func OutcomeUnknown(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.outcomeUnknown()
	}
	return errors.As(err, new(*OutcomeUnknownError))
}

// unknownOutcomeText says that the change may have been applied and which
// command shows the state.
func unknownOutcomeText(path string) string {
	if cmd := stateCommand(path); cmd != "" {
		return "the change may have been applied; check it with " + cmd + " before running this again"
	}
	return "the change may have been applied; check its state before running this again"
}

// stateCommand is the command that shows what the API path's resource looks
// like now, or "" when the path names no resource the CLI can show. The path
// is one of the Astro API's, under /organizations/{id}.
func stateCommand(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	// Drop everything up to and including the organization ID.
	for i, s := range segs {
		if s == "organizations" && i+1 < len(segs) {
			segs = segs[i+2:]
			break
		}
	}
	const fallback = ""
	if len(segs) == 0 {
		return fallback
	}
	switch segs[0] {
	case "deployments":
		if len(segs) > 1 {
			return "astro deployment inspect " + segs[1]
		}
		return "astro deployment list"
	case "workspaces":
		return "astro workspace list"
	case "teams":
		return "astro organization team list"
	case "tokens":
		return "astro organization token list"
	case "users", "invites":
		return "astro organization user list"
	case "clusters":
		return "astro organization cluster list"
	case "environment-objects":
		return "astro env list"
	}
	return fallback
}

// markOutcomeUnknown is what a transport failure of a request becomes when the
// request changes state and may have been sent. A failure before anything was
// sent (the host could not be reached) is left as it is: nothing happened.
func markOutcomeUnknown(req *http.Request, err error) error {
	if err == nil || !isMutation(req.Method) {
		return err
	}
	// The failure without any *url.Error the retrying transport left on it, so
	// the client's own wrapping adds the URL once.
	inner := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		inner = urlErr.Err
	}
	// IsTransportError reads the *url.Error an http.Client wraps a failure in.
	// A request canceled or timed out once it was sent is as unknown as one
	// whose connection broke: the server may have acted before it heard.
	probe := error(&url.Error{Op: req.Method, URL: req.URL.String(), Err: inner})
	ended := errors.Is(inner, context.Canceled) || errors.Is(inner, context.DeadlineExceeded)
	if (!IsTransportError(probe) && !ended) || notSent(inner) {
		return err
	}
	return &OutcomeUnknownError{Method: req.Method, Path: req.URL.Path, Err: inner}
}

// notSent reports that err happened before the request left: the connection
// was never made, so the server cannot have acted.
func notSent(err error) bool {
	for _, errno := range []error{
		syscall.ECONNREFUSED, syscall.ENETUNREACH, syscall.EHOSTUNREACH,
		wsaENETUNREACH, wsaECONNREFUSED, wsaEHOSTUNREACH,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}
	var (
		dnsErr *net.DNSError
		opErr  *net.OpError
	)
	return errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial")
}

// methodOfTransportError is the HTTP method a transport failure came from,
// read off the *url.Error the client wraps it in.
func methodOfTransportError(err error) (string, bool) {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr.Op == "" {
		return "", false
	}
	return strings.ToUpper(urlErr.Op), true
}

// outcomeTransport marks the transport failures of requests that change state
// (markOutcomeUnknown).
type outcomeTransport struct{ next http.RoundTripper }

func (t outcomeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A request whose context had already ended was never sent.
	sendable := req.Context().Err() == nil
	resp, err := t.next.RoundTrip(req)
	if err != nil && sendable {
		err = markOutcomeUnknown(req, err)
	}
	return resp, err
}
