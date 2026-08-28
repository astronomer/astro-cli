package airflowapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// The conditions callers branch on. A *StatusError unwraps to whichever one
// matches its status, so errors.Is answers without a type assertion.
var (
	// ErrNotFound reports a 404: the dag, run, or task instance is not there.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized reports a 401: no credential, or one Airflow rejected.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden reports a 403: authenticated, but not allowed. Airflow 2
	// answers this way for /config unless expose_config is on.
	ErrForbidden = errors.New("forbidden")
	// ErrNotServed reports that this Airflow does not serve the endpoint at
	// all. What an instance has is discovered by asking — a 405 anywhere, or
	// a 404 from a list endpoint, whose path does not depend on a dag or run
	// existing — never tabulated against a minor version. A caller with
	// something else to show branches here.
	ErrNotServed = errors.New("not served by this airflow")
	// ErrHeadersUnsupported reports a door with nowhere to put Request.Header
	// — MWAA's InvokeRestApi, which takes a path, a method, a query, and a
	// body and nothing else. A transport that cannot carry headers wraps this
	// rather than dropping them silently (see the Transport contract).
	//
	// It is a sentinel so a caller whose header is a preference rather than a
	// requirement can ask again without it. Only Client.TaskLogs does that
	// today: text/plain makes an Airflow 2 log readable, and where it cannot
	// be asked for, the log still arrives in the shape it always had.
	ErrHeadersUnsupported = errors.New("this door carries no request headers")
)

// maxBodyInError bounds how much of an unexplained error body reaches the
// message; the whole body stays on the struct.
const maxBodyInError = 200

// StatusError reports a non-2xx answer from Airflow. Body is what the server
// said, kept whole so a caller can render it.
type StatusError struct {
	// Method and Path are the call as the caller made it: Path is
	// API-relative, without the version prefix.
	Method string
	Path   string
	// StatusCode is the HTTP status, or the status a non-HTTP transport
	// reports for the wrapped call.
	StatusCode int
	// Body is the raw error payload.
	Body []byte
	// Collection marks a call to a list endpoint, whose path names no dag or
	// run that could be missing. There a 404 means this Airflow does not
	// serve the endpoint, so the error reads as ErrNotServed as well as
	// ErrNotFound and a caller can degrade instead of failing.
	Collection bool
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("%s %s: airflow returned %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	if detail := e.Detail(); detail != "" {
		return msg + ": " + detail
	}
	if body := strings.TrimSpace(string(e.Body)); body != "" {
		return msg + ": " + truncate(body, maxBodyInError)
	}
	return msg
}

func (e *StatusError) Unwrap() []error {
	switch e.StatusCode {
	case http.StatusNotFound:
		if e.Collection {
			// Ambiguous by nature: this Airflow has no such endpoint, or a
			// dag named further up the path is gone. It matches both, so
			// either branch can act.
			return []error{ErrNotFound, ErrNotServed}
		}
		return []error{ErrNotFound}
	case http.StatusMethodNotAllowed:
		return []error{ErrNotServed}
	case http.StatusUnauthorized:
		return []error{ErrUnauthorized}
	case http.StatusForbidden:
		return []error{ErrForbidden}
	}
	return nil
}

// Detail is Airflow's own explanation, read from the "detail" field it puts
// in error bodies. It is "" when the body is some other shape.
func (e *StatusError) Detail() string {
	var payload struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(e.Body, &payload); err != nil || len(payload.Detail) == 0 {
		return ""
	}
	// Airflow 2 puts a sentence here; Airflow 3's validation errors put a
	// list of field problems.
	var text string
	if err := json.Unmarshal(payload.Detail, &text); err == nil {
		return text
	}
	return truncate(strings.TrimSpace(string(payload.Detail)), maxBodyInError)
}

// statusError turns a non-2xx answer about a named object into a
// *StatusError.
func statusError(method, path string, resp Response) error {
	return newStatusError(method, path, resp, false)
}

// collectionError turns a non-2xx answer from a list endpoint into a
// *StatusError that also reads as ErrNotServed on a 404.
func collectionError(method, path string, resp Response) error {
	return newStatusError(method, path, resp, true)
}

func newStatusError(method, path string, resp Response, collection bool) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if method == "" {
		method = http.MethodGet
	}
	return &StatusError{
		Method:     method,
		Path:       path,
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
		Collection: collection,
	}
}

// ProbeResult is one generation's answer during detection.
type ProbeResult struct {
	Generation Generation
	Err        error
}

// DetectError reports that no generation answered the version probe. It keeps
// both answers because they mean different things: a refused connection is an
// Airflow that is not running, a 401 is credentials, and a 404 from both is
// something other than Airflow at that URL.
type DetectError struct {
	Probes []ProbeResult
}

func (e *DetectError) Error() string {
	reasons := make([]string, 0, len(e.Probes))
	for _, probe := range e.Probes {
		reasons = append(reasons, fmt.Sprintf("%s/version: %v", probe.Generation.BasePath(), probe.Err))
	}
	if len(reasons) == 0 {
		return "cannot tell which airflow api generation this is: nothing answered"
	}
	return "cannot tell which airflow api generation this is: " + strings.Join(reasons, "; ")
}

// Unwrap exposes the probe failures, so errors.Is finds the condition behind
// a detection failure.
//
// A refused credential wins on its own: an Airflow 3 that answers 401 on
// /api/v2 answers 404 on /api/v1 too, and reporting both would let a caller
// checking not-found first conclude there is no Airflow there at all.
func (e *DetectError) Unwrap() []error {
	refused := make([]error, 0, len(e.Probes))
	for _, probe := range e.Probes {
		if errors.Is(probe.Err, ErrUnauthorized) || errors.Is(probe.Err, ErrForbidden) {
			refused = append(refused, probe.Err)
		}
	}
	if len(refused) > 0 {
		return refused
	}
	errs := make([]error, 0, len(e.Probes))
	for _, probe := range e.Probes {
		errs = append(errs, probe.Err)
	}
	return errs
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
