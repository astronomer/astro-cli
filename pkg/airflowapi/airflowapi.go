// Package airflowapi talks to the Airflow REST API, adapting to the
// generation the server speaks. Airflow 3 serves /api/v2 and Airflow 2 serves
// /api/v1, and the two disagree about more than the prefix: datasets became
// assets, execution_date became logical_date, DAG source is addressed by file
// token in one and by dag id in the other. Each operation here handles that
// difference so its caller does not have to.
//
// # Shape
//
// A Client is a Transport plus the API generation, detected once on first use
// and cached:
//
//	transport, err := airflowapi.NewHTTPTransport("http://localhost:8080",
//		airflowapi.WithCredentials(airflowapi.BearerToken(token)))
//	client := airflowapi.New(transport)
//	dags, err := client.ListDAGs(ctx, airflowapi.ListDAGsOptions{})
//
// Transport is an interface because not every door is HTTP to a URL — MWAA's
// InvokeRestApi wraps the request in a signed AWS API call — so the common
// case (a base URL plus a credential source) is one implementation among
// several rather than the only shape.
//
// # Errors
//
// Nothing here prints or exits. A non-2xx answer becomes a *StatusError
// carrying the body, and errors.Is against ErrNotFound, ErrUnauthorized,
// ErrForbidden, and ErrNotServed is how a caller branches. ErrNotServed is
// the one to reach for when a command has something else to show: it means
// this instance does not have the endpoint at all, which is how the client
// learns what an Airflow can do — by asking, never from a version table.
//
// Do and DoRoot, the raw passthrough, are the exception to all of it: they
// hand back whatever status came so a passthrough command can report it
// faithfully.
package airflowapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// VersionInfo is Airflow's own version and the generation serving it. Every
// field serializes, so a command can render it as it stands.
type VersionInfo struct {
	// Version is the full version string, e.g. "3.1.3" or "2.10.5+astro.4".
	Version string `json:"version"`
	// GitVersion is the build Airflow reports, often empty.
	GitVersion string `json:"git_version"`
	// Generation is the API generation this client talks to.
	Generation Generation `json:"generation"`
}

// versionWire is the /version payload. It is separate from VersionInfo so
// the public type can carry the generation without the wire shape claiming
// Airflow sent it.
type versionWire struct {
	Version    string `json:"version"`
	GitVersion string `json:"git_version"`
}

func (w versionWire) info(generation Generation) VersionInfo {
	return VersionInfo{Version: w.Version, GitVersion: w.GitVersion, Generation: generation}
}

// Client is a generation-adaptive Airflow API client. It is safe for
// concurrent use; the generation is resolved once and shared.
type Client struct {
	transport Transport
	pinned    Generation

	mu sync.Mutex
	// info is the resolved generation, valid once detected is set. fetched
	// says the /version payload was read too, which a pinned generation
	// skips.
	info      VersionInfo
	detected  bool
	fetched   bool
	detecting *detection
}

// detection is one run of the version probe. The caller that starts it fills
// the result and closes done; everyone who arrives meanwhile waits on done
// rather than probing again, and can give up on its own context instead.
type detection struct {
	done chan struct{}
	info VersionInfo
	err  error
}

// Option configures a Client.
type Option func(*Client)

// WithGeneration pins the generation and skips detection, for a caller that
// already knows — a local Airflow's state record carries its major version.
func WithGeneration(g Generation) Option {
	return func(c *Client) { c.pinned = g }
}

// New builds a client over a transport.
func New(transport Transport, opts ...Option) *Client {
	c := &Client{transport: transport}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Generation reports the generation this client talks to, detecting it on
// first use.
func (c *Client) Generation(ctx context.Context) (Generation, error) {
	info, err := c.detect(ctx)
	return info.Generation, err
}

// Version reports Airflow's own version and its generation. Detection reads
// the same endpoint, so this costs nothing after the first call.
//
// The answer is cached for the life of the client. A long-lived process that
// holds a client across an Airflow restart — desktop does — gets a new client
// when it wants a fresh answer.
func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	info, err := c.detect(ctx)
	if err != nil {
		return VersionInfo{}, err
	}
	// Read both together: another caller may have filled the version in
	// between the detection above and this check.
	c.mu.Lock()
	cached, fetched := c.info, c.fetched
	c.mu.Unlock()
	if fetched {
		return cached, nil
	}

	// The generation was pinned, so the version endpoint was never read.
	wire, err := c.readVersion(ctx, info.Generation)
	if err != nil {
		return VersionInfo{}, err
	}
	full := wire.info(info.Generation)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info, c.fetched = full, true
	return full, nil
}

// detect resolves the generation once. Only a success is cached, so an
// Airflow that was down does not poison the client. Concurrent callers share
// one probe and each keeps its own deadline: waiting happens on a channel,
// never on a mutex held across the network.
func (c *Client) detect(ctx context.Context) (VersionInfo, error) {
	c.mu.Lock()
	if c.detected {
		defer c.mu.Unlock()
		return c.info, nil
	}
	if c.pinned != GenerationNone {
		defer c.mu.Unlock()
		c.info, c.detected = VersionInfo{Generation: c.pinned}, true
		return c.info, nil
	}
	if running := c.detecting; running != nil {
		c.mu.Unlock()
		select {
		case <-running.done:
			return running.info, running.err
		case <-ctx.Done():
			return VersionInfo{}, ctx.Err()
		}
	}
	running := &detection{done: make(chan struct{})}
	c.detecting = running
	c.mu.Unlock()

	running.info, running.err = c.probe(ctx)

	c.mu.Lock()
	if running.err == nil {
		c.info, c.detected, c.fetched = running.info, true, true
	}
	c.detecting = nil
	c.mu.Unlock()
	close(running.done)
	return running.info, running.err
}

// probe asks each generation for its version, newest first — the order
// pkg/airflowrt probes health in. An Airflow 3 has no /api/v1 at all, while
// an Astronomer-patched Airflow 2 answers on some /api/v2 paths, so the
// version it reports decides rather than the probe that answered.
func (c *Client) probe(ctx context.Context) (VersionInfo, error) {
	probes := make([]ProbeResult, 0, 2)
	for _, candidate := range []Generation{Airflow3, Airflow2} {
		wire, err := c.readVersion(ctx, candidate)
		if err != nil {
			probes = append(probes, ProbeResult{Generation: candidate, Err: err})
			continue
		}
		return wire.info(generationOf(wire.Version, candidate)), nil
	}
	return VersionInfo{}, &DetectError{Probes: probes}
}

func (c *Client) readVersion(ctx context.Context, g Generation) (versionWire, error) {
	const path = "/version"
	resp, err := c.transport.Do(ctx, Request{Method: http.MethodGet, Path: path, Generation: g})
	if err != nil {
		return versionWire{}, err
	}
	if err := statusError(http.MethodGet, path, resp); err != nil {
		return versionWire{}, err
	}
	var wire versionWire
	if err := resp.Decode(&wire); err != nil {
		return versionWire{}, err
	}
	return wire, nil
}

// airflow3Major is the first Airflow major version whose API is /api/v2.
const airflow3Major = 3

// generationOf reads the API generation off Airflow's reported version, so an
// Astronomer-patched Airflow 2 that answers an /api/v2 probe still gets the
// /api/v1 paths its API actually has. An unparseable version falls back to
// whichever probe answered.
func generationOf(version string, probe Generation) Generation {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	switch {
	case err != nil:
		return probe
	case n >= airflow3Major:
		return Airflow3
	default:
		return Airflow2
	}
}

// Do sends a request against the detected generation, whatever path the
// caller names, and hands back the answer whatever its status. This is the
// passthrough for endpoints with no typed operation here; it is the one place
// a non-2xx is not an error, because a passthrough command reports the status
// itself.
func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	info, err := c.detect(ctx)
	if err != nil {
		return Response{}, err
	}
	req.Generation = info.Generation
	return c.transport.Do(ctx, req)
}

// DoRoot sends a request to the server root, below any API prefix, for the
// paths Airflow serves unversioned: /health on Airflow 2, /openapi.json on
// Airflow 3. Like Do, it returns the answer whatever its status.
func (c *Client) DoRoot(ctx context.Context, req Request) (Response, error) {
	req.Generation = GenerationNone
	return c.transport.Do(ctx, req)
}

// call sends a request against the detected generation and turns a non-2xx
// into a *StatusError. Every typed operation goes through here or through
// callCollection.
func (c *Client) call(ctx context.Context, req Request) (Response, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if err := statusError(req.Method, req.Path, resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// callCollection is call for a list endpoint, where a 404 also reads as
// ErrNotServed: an instance too old for a collection has no such path.
func (c *Client) callCollection(ctx context.Context, req Request) (Response, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if err := collectionError(req.Method, req.Path, resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// send is call without detection, for the endpoints a caller addresses by
// generation itself.
func (c *Client) send(ctx context.Context, req Request) (Response, error) {
	resp, err := c.transport.Do(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if err := statusError(req.Method, req.Path, resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// do is call plus decoding; a nil out discards the body.
func (c *Client) do(ctx context.Context, req Request, out any) error {
	resp, err := c.call(ctx, req)
	return decoded(resp, err, out)
}

// get is the shape most operations have: a GET with a query, decoded.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, Request{Method: http.MethodGet, Path: path, Query: query}, out)
}

// getCollection is get for a list endpoint. See callCollection.
func (c *Client) getCollection(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.callCollection(ctx, Request{Method: http.MethodGet, Path: path, Query: query})
	return decoded(resp, err, out)
}

func decoded(resp Response, err error, out any) error {
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return resp.Decode(out)
}

// DefaultLimit is the page size a list uses when its options name none. It
// is sent explicitly rather than left to the server: the two generations
// default differently (Airflow 2 to 100, Airflow 3 to 50), and a page size
// that changes with the instance is a surprise a command should not carry.
const DefaultLimit = 100

// ListOptions is the pagination every list endpoint takes.
type ListOptions struct {
	// Limit is the page size; zero means DefaultLimit.
	Limit int
	// Offset is where the page starts.
	Offset int
	// OrderBy is an Airflow sort field, e.g. "-start_date". Its accepted
	// values differ per endpoint and per generation.
	OrderBy string
}

func (o ListOptions) query() url.Values {
	query := url.Values{}
	limit := o.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	query.Set("limit", strconv.Itoa(limit))
	if o.Offset > 0 {
		query.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.OrderBy != "" {
		query.Set("order_by", o.OrderBy)
	}
	return query
}
