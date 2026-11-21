package httputil

import (
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
	return "the change may have been applied; check it with " + stateCommand(path) + " before running this again"
}

// stateCommand is the command that shows what the API path's resource looks
// like now. The path is one of the Astro API's, under /organizations/{id}.
func stateCommand(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	// Drop everything up to and including the organization ID.
	for i, s := range segs {
		if s == "organizations" && i+1 < len(segs) {
			segs = segs[i+2:]
			break
		}
	}
	const fallback = "the matching list or inspect command"
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
	// The retrying transport hands over the bare failure; IsTransportError
	// reads the *url.Error an http.Client wraps it in.
	probe := error(&url.Error{Op: req.Method, URL: req.URL.String(), Err: err})
	if !IsTransportError(probe) || notSent(err) {
		return err
	}
	return &OutcomeUnknownError{Method: req.Method, Path: req.URL.Path, Err: err}
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
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		err = markOutcomeUnknown(req, err)
	}
	return resp, err
}
