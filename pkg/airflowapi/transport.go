package airflowapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Generation is an Airflow REST API generation.
type Generation int

const (
	// GenerationNone addresses the server root, below any API prefix. It is
	// the zero value so a bare Request reaches paths Airflow serves
	// unversioned, such as /health.
	GenerationNone Generation = iota
	// Airflow2 is Airflow 2, which serves /api/v1.
	Airflow2
	// Airflow3 is Airflow 3, which serves /api/v2.
	Airflow3
)

// BasePath is the prefix every request of this generation hangs under.
func (g Generation) BasePath() string {
	switch g {
	case Airflow2:
		return "/api/v1"
	case Airflow3:
		return "/api/v2"
	case GenerationNone:
		return ""
	}
	return ""
}

// String is the Airflow major version, or "root" for GenerationNone.
func (g Generation) String() string {
	switch g {
	case Airflow2:
		return "2"
	case Airflow3:
		return "3"
	case GenerationNone:
		return "root"
	}
	return "unknown"
}

// MarshalJSON writes the generation as its name, so a rendered VersionInfo
// says "3" rather than an internal number.
func (g Generation) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.String())
}

// UnmarshalJSON reads what MarshalJSON wrote.
func (g *Generation) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("decode airflow generation: %w", err)
	}
	switch name {
	case "2":
		*g = Airflow2
	case "3":
		*g = Airflow3
	default:
		*g = GenerationNone
	}
	return nil
}

// Request is one Airflow REST API call. Path is relative to the API base —
// "/dags" reaches /api/v2/dags on Airflow 3 and /api/v1/dags on Airflow 2 —
// so a transport that is not HTTP-to-a-URL receives the path its own door
// expects. Generation picks the base.
type Request struct {
	// Method is an HTTP method; empty means GET.
	Method string
	// Path is relative to the API base, with or without a leading slash.
	// With Generation GenerationNone it is relative to the server root.
	Path string
	// Generation selects the API generation serving Path. Client fills this
	// in from the generation it detected, so callers rarely set it.
	Generation Generation
	// Query is appended as the query string.
	Query url.Values
	// Body is JSON-encoded when non-nil. Nil sends no body at all, which is
	// not the same as an empty object.
	Body any
	// Header is merged over the transport's own headers, and wins where they
	// overlap — including Authorization, which a caller may set to override
	// the transport's credentials for one request.
	Header http.Header
}

// Response is what a Transport got back, whatever the status. Turning a
// non-2xx into an error is the Client's job, so every transport reports a
// status the same way.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	// Proto is the protocol the answer came over, "HTTP/1.1" and the like, for
	// a caller that prints the status line. Empty from a door that is not HTTP
	// to the Airflow itself.
	Proto string
}

// Decode unmarshals the body into v. An empty body leaves v untouched, so a
// 204 decodes to the zero value rather than an error.
//
// A body that is plainly not JSON is named as such. Something in front of
// Airflow — a login page, a proxy's error page — answering 200 with HTML is
// the common cause, and "invalid character '<'" sends the reader hunting
// through their DAGs instead of their ingress.
func (r Response) Decode(v any) error {
	body := bytes.TrimSpace(r.Body)
	if len(body) == 0 {
		return nil
	}
	if !looksLikeJSON(r.Header, body) {
		return fmt.Errorf("airflow answered with %s, not the JSON its API returns — check what is in front of it", describeBody(r.Header, body))
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode airflow response: %w", err)
	}
	return nil
}

// looksLikeJSON trusts a JSON content type, and otherwise judges by the first
// byte: Airflow serves JSON without a content type in a few places, and a
// login page never starts with a brace.
func looksLikeJSON(header http.Header, body []byte) bool {
	if mediaType(header) == jsonMediaType {
		return true
	}
	switch body[0] {
	case '{', '[', '"':
		return true
	}
	return false
}

func describeBody(header http.Header, body []byte) string {
	if kind := mediaType(header); kind != "" {
		return kind
	}
	if bytes.HasPrefix(body, []byte("<")) {
		return "an HTML page"
	}
	return "something other than JSON"
}

func mediaType(header http.Header) string {
	kind, _, _ := strings.Cut(header.Get("Content-Type"), ";")
	return strings.ToLower(strings.TrimSpace(kind))
}

// Transport executes Airflow REST API requests. It is an interface rather
// than a URL and a token because not every door is HTTP to an Airflow:
// MWAA's InvokeRestApi wraps the request in a signed AWS API call with no
// Airflow URL in sight.
//
// What an implementation owes its caller:
//
//   - Address Request.Generation.BasePath() + Request.Path. A door that
//     already speaks one generation — InvokeRestApi takes the unprefixed
//     path and adds its own — uses Path alone and ignores the prefix.
//   - Answer a GenerationNone request, which addresses the server root
//     below the API, with 404 when there is no server root to reach. The
//     client reads that as ErrNotServed and moves on, so a door with no
//     unversioned paths needs no special case anywhere else.
//   - Report the status in Response.StatusCode and never as an error. A
//     returned error means the request did not complete: no connection, no
//     credentials, a refusal by the door itself. Turning a status into a
//     typed error is the Client's job, so every door reports alike.
//   - Apply Request.Header over its own headers, Authorization included. An
//     implementation with nowhere to put them — MWAA's InvokeRestApi takes a
//     path, a method, a query, and a body, and no headers at all — returns an
//     error wrapping ErrHeadersUnsupported and naming what it cannot carry,
//     rather than dropping it. Authorization is the header a caller is most
//     likely to set and this contract promises it wins; sending the request as
//     somebody else instead would be a surprise nobody could see. The sentinel
//     lets a caller whose header was only a preference ask again without it.
type Transport interface {
	Do(ctx context.Context, req Request) (Response, error)
}

// CredentialSource produces the credential for one request: an
// Authorization scheme and its value, or an empty scheme to send the request
// unauthenticated. It runs per request and the transport stores nothing, so
// a caller can hand over a session that refreshes underneath it.
type CredentialSource func(ctx context.Context) (scheme, value string, err error)

// The Authorization schemes this package produces.
const (
	bearerScheme = "Bearer"
	basicScheme  = "Basic"
)

// BearerCredential is the credential in a bearer token as the CLI holds one:
// a stored session token, which carries its scheme ("Bearer <token>"), or one
// supplied bare, as ASTRO_API_TOKEN is. It is the one rule every reader of
// such a token uses, so one run cannot read a credential where another reads
// none.
//
// The scheme is read as HTTP reads it: in any case, set off from the token by
// any run of spaces or tabs, with space around both ignored. It is stripped
// however often it repeats ("Bearer Bearer <token>"), and the scheme with
// nothing after it ("Bearer ", "Bearer Bearer") is no credential: "" .
func BearerCredential(token string) string {
	fields := strings.Fields(token)
	for len(fields) > 0 && strings.EqualFold(fields[0], bearerScheme) {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

// BearerToken is a CredentialSource for a token that is already in hand, read
// through BearerCredential: the CLI's stored session tokens carry the scheme
// in the value, and sending it twice is a 401 nobody can read.
func BearerToken(token string) CredentialSource {
	value := BearerCredential(token)
	return func(context.Context) (string, string, error) {
		return bearerScheme, value, nil
	}
}

// BasicAuth is a CredentialSource for username and password, which Airflow 2
// takes directly. On an Airflow that mints tokens, use a TokenMinter.
func BasicAuth(username, password string) CredentialSource {
	value := basicValue(username, password)
	return func(context.Context) (string, string, error) {
		return basicScheme, value, nil
	}
}

func basicValue(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// defaultTimeout bounds one request. Airflow's slowest ordinary answers are
// task logs and a cold /config, both well inside this.
const defaultTimeout = 30 * time.Second

// jsonMediaType is what the API answers with, and what every request asks for
// unless it says otherwise. textMediaType is the one exception: Airflow 2
// serves a task log as text to a caller who asks for text (Client.TaskLogs).
const (
	jsonMediaType = "application/json"
	textMediaType = "text/plain"
)

// HTTPTransport is the common Transport: a base URL plus a credential source
// applied to every request.
type HTTPTransport struct {
	baseURL string
	client  *http.Client
	creds   CredentialSource
	refresh func(ctx context.Context) error
}

// HTTPOption configures an HTTPTransport.
type HTTPOption func(*HTTPTransport)

// WithHTTPClient replaces the default client, whose only setting is a
// timeout. A caller that needs a proxy, a custom CA, or no TLS verification
// builds the client itself.
func WithHTTPClient(client *http.Client) HTTPOption {
	return func(t *HTTPTransport) { t.client = client }
}

// WithCredentials sets what proves the caller's identity on every request.
func WithCredentials(src CredentialSource) HTTPOption {
	return func(t *HTTPTransport) { t.creds = src }
}

// WithRefresh installs a hook called once when a request comes back 401 or
// 403 — Airflow 3's bearer handler answers 403 to a malformed or missing
// token, so both mean "try new credentials". The hook renews whatever the
// CredentialSource reads and the request is sent a second time; a hook that
// fails ends the request with its error. This is the only retry the transport
// does.
func WithRefresh(fn func(ctx context.Context) error) HTTPOption {
	return func(t *HTTPTransport) { t.refresh = fn }
}

// NewHTTPTransport builds a transport against an Airflow base URL. See
// normalizeBaseURL for the shapes it accepts.
func NewHTTPTransport(baseURL string, opts ...HTTPOption) (*HTTPTransport, error) {
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	t := &HTTPTransport{baseURL: normalized, client: DefaultHTTPClient()}
	for _, opt := range opts {
		opt(t)
	}
	return t, nil
}

// DefaultHTTPClient is the client an HTTPTransport uses when a caller names
// none: a timeout, and redirects refused.
//
// Refusing them matters. An Airflow 2 behind session auth answers an
// unauthenticated API call with 302 to its login page, and a client that
// follows lands on 200 HTML — a rejection wearing a success, which no caller
// could branch on. Stopping at the first hop keeps the 302, which reads as
// ErrUnauthorized.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// BaseURL is the URL this transport addresses, after normalizing: a scheme
// added where the caller's had none, any trailing slash and API prefix removed.
//
// It exists for a caller that builds its own requests rather than sending them
// through Do — `astro api airflow`, which generates curl commands and rewrites
// query strings — so that caller addresses the same Airflow the transport
// would, instead of re-deriving it from the raw string and getting a URL with
// no scheme.
func (t *HTTPTransport) BaseURL() string { return t.baseURL }

func (t *HTTPTransport) Do(ctx context.Context, req Request) (Response, error) {
	body, err := encodeBody(req.Body)
	if err != nil {
		return Response{}, err
	}
	resp, err := t.send(ctx, req, body)
	if err != nil || !refreshable(resp.StatusCode) || t.refresh == nil {
		return resp, err
	}
	if err := t.refresh(ctx); err != nil {
		return Response{}, fmt.Errorf("airflow rejected the credentials and refreshing them failed: %w", err)
	}
	return t.send(ctx, req, body)
}

// refreshable is the pair of answers that mean "these credentials did not
// work", which is the one thing a refresh can fix.
func refreshable(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func (t *HTTPTransport) send(ctx context.Context, req Request, body []byte) (Response, error) {
	target := t.baseURL + req.Generation.BasePath() + leadingSlash(req.Path)
	if len(req.Query) > 0 {
		target += "?" + req.Query.Encode()
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return Response{}, fmt.Errorf("build request for %s %s: %w", method, target, err)
	}
	httpReq.Header.Set("Accept", jsonMediaType)
	if body != nil {
		httpReq.Header.Set("Content-Type", jsonMediaType)
	}
	for key, values := range req.Header {
		httpReq.Header[http.CanonicalHeaderKey(key)] = values
	}
	// The caller's headers win, Authorization included: a passthrough command
	// carrying its own credential means it.
	if t.creds != nil && httpReq.Header.Get("Authorization") == "" {
		scheme, value, err := t.creds(ctx)
		if err != nil {
			return Response{}, fmt.Errorf("resolve credentials: %w", err)
		}
		if scheme != "" {
			httpReq.Header.Set("Authorization", scheme+" "+value)
		}
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("call airflow at %s: %w", target, err)
	}
	defer httpResp.Body.Close()
	// Read whole: task logs are the largest answer and truncating one would
	// be worse than the memory.
	payload, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read airflow response from %s: %w", target, err)
	}
	return Response{
		StatusCode: redirectAsRefusal(httpResp),
		Header:     httpResp.Header,
		Body:       payload,
		Proto:      httpResp.Proto,
	}, nil
}

// redirectAsRefusal reads a redirect off the API as the rejection it is. An
// Airflow 2 under session auth bounces an unauthenticated API call to its
// login page; the hop itself is the refusal, so it reports 401 rather than a
// 302 no caller would branch on. A redirect that stays on the API path — a
// trailing-slash normalization — keeps its own status.
func redirectAsRefusal(resp *http.Response) int {
	if resp.StatusCode < http.StatusMultipleChoices || resp.StatusCode >= http.StatusBadRequest {
		return resp.StatusCode
	}
	// No Location is not a redirect anywhere — a 304, or something
	// malformed. It keeps its own status.
	target, err := resp.Location()
	if err != nil {
		return resp.StatusCode
	}
	if strings.Contains(target.Path, "/api/") {
		return resp.StatusCode
	}
	return http.StatusUnauthorized
}

func encodeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	return encoded, nil
}

// normalizeBaseURL reduces the shapes callers hold to one this package can
// append paths to. A base URL keeps its path prefix, since deployments are
// often served under one, and loses:
//
//   - the query string and fragment, so a stored
//     https://host/dep?orgId=x does not build https://host/dep?orgId=x/api/v2/dags,
//   - every trailing slash, so host:8080// does not build host:8080//api/v2/dags,
//   - a trailing /api/v1 or /api/v2, which is how an Airflow API URL is
//     usually written down and would otherwise be prefixed twice.
//
// A URL with no scheme is read as https, the shape a control plane hands back.
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("airflow url is empty")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse airflow url %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("airflow url %q needs an http or https scheme", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("airflow url %q has no host", raw)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	for _, base := range []string{Airflow3.BasePath(), Airflow2.BasePath()} {
		if strings.HasSuffix(parsed.Path, base) {
			parsed.Path = strings.TrimSuffix(parsed.Path, base)
			break
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func leadingSlash(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

// pathf builds an API path, escaping every interpolated id so a dag or run
// id carrying a slash cannot reshape the URL.
func pathf(format string, ids ...string) string {
	escaped := make([]any, len(ids))
	for i, id := range ids {
		escaped[i] = url.PathEscape(id)
	}
	return fmt.Sprintf(format, escaped...)
}
