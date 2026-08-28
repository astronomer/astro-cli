package instances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/mwaa"
	"github.com/aws/aws-sdk-go-v2/service/mwaa/document"
	mwaatypes "github.com/aws/aws-sdk-go-v2/service/mwaa/types"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// There are two ways into an MWAA environment. This file takes the first:
// InvokeRestApi, an AWS API that carries an Airflow REST call inside a
// SigV4-signed request — no Airflow URL, no Airflow credential, just the
// caller's ordinary AWS identity and the airflow:InvokeRestAPI permission. It
// is the one to prefer: nothing is minted, nothing expires mid-command, and the
// IAM policy is the whole access story.
//
// The second, in mwaaweblogin.go, is the older web-login-token exchange, for
// environments whose IAM policy predates the first. Amazon's own Airflow
// provider tries the two in this order and so does this.
//
// Which one works is discovered by asking: the fallback is built the first time
// InvokeRestApi is refused, and every request after that goes through it.

// awsRegionKey is the [tool.astro.targets.mwaa] field naming the region an
// environment lives in. Nothing else in that section is read here — the bucket
// belongs to deploy.
const awsRegionKey = "region"

// mwaaAPI is the slice of the MWAA client this package uses. It is an interface
// so both ways in can be driven in a test without an AWS account, and so the
// two calls made here are the two that can be made.
type mwaaAPI interface {
	InvokeRestApi(ctx context.Context, params *mwaa.InvokeRestApiInput, optFns ...func(*mwaa.Options)) (*mwaa.InvokeRestApiOutput, error)
	CreateWebLoginToken(ctx context.Context, params *mwaa.CreateWebLoginTokenInput, optFns ...func(*mwaa.Options)) (*mwaa.CreateWebLoginTokenOutput, error)
}

// errNoAWSCredentials reports a machine whose AWS credential chain resolves
// nothing. Every way of having no AWS identity — no profile, an SSO session
// that lapsed, an expired assumed role — ends here, because the fixes are the
// two named.
var errNoAWSCredentials = errors.New("no AWS credentials found — run `aws sso login`, or set AWS_PROFILE")

// awsCredentialTimeout bounds the up-front walk of the credential chain.
const awsCredentialTimeout = 15 * time.Second

// awsCredentialFailure tells a chain that answered "nobody" from one that did
// not answer in time. They read the same to the SDK and mean opposite things to
// the person waiting: one is a machine with no AWS identity, the other is a
// credential helper still working — an SSO login in a browser tab, a
// credential_process asking for a fingerprint — and telling them to log in
// again would be advice to abandon the login they are in the middle of.
func awsCredentialFailure(err error, environment, region string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("your AWS credentials did not arrive within %s (reaching %s in %s) — an `aws sso login` or a credential_process helper may still be waiting on you; finish it and run this again",
			awsCredentialTimeout, environment, region)
	}
	return fmt.Errorf("%w (reaching %s in %s): %w", errNoAWSCredentials, environment, region, err)
}

// mwaaTransport opens the way to an MWAA environment: an SDK client on the
// caller's own credential chain, and the environment name from the link.
//
// The credential chain is resolved here rather than at the first request so a
// machine with no AWS identity is told so plainly, once, instead of receiving
// the SDK's own wording wrapped around whichever provider failed last.
func (i Instance) mwaaTransport(ctx context.Context, d Deps) (airflowapi.Transport, error) {
	environment := i.Link.Environment
	if environment == "" {
		return nil, fmt.Errorf("deployment %q names no MWAA environment: set environment = '<environment name>' on the link", i.Name)
	}
	region, err := i.TargetString(awsRegionKey)
	if err != nil {
		return nil, err
	}
	cfg, err := d.awsConfig(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("read the AWS configuration for %q: %w", i.Name, err)
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("no AWS region for %q: add region = '<region>' under [tool.astro.targets.mwaa], or set AWS_REGION", i.Name)
	}
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("%w (reaching %s in %s)", errNoAWSCredentials, environment, cfg.Region)
	}
	// Bounded, because the last provider in the chain is the EC2 instance
	// metadata service: on a laptop that is not an EC2 instance it answers
	// nothing and the SDK spends its own retries finding that out, which is a
	// long wait for the word "no". The bound is generous because the chain also
	// holds credential_process, and a helper that opens a browser or asks for a
	// fingerprint is working, not broken.
	preflight, cancel := context.WithTimeout(ctx, awsCredentialTimeout)
	defer cancel()
	if _, err := cfg.Credentials.Retrieve(preflight); err != nil {
		return nil, awsCredentialFailure(err, environment, cfg.Region)
	}
	return &awsTransport{
		api:         mwaa.NewFromConfig(cfg),
		environment: environment,
		region:      cfg.Region,
		base:        d.baseHTTPClient(),
	}, nil
}

// awsConfig loads the AWS SDK configuration, honoring the region the manifest
// named and otherwise letting the SDK's own chain decide.
func (d Deps) awsConfig(ctx context.Context, region string) (aws.Config, error) {
	if d.AWSConfig != nil {
		return d.AWSConfig(ctx, region)
	}
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

// awsTransport reaches an MWAA environment. It starts on InvokeRestApi and
// moves to the web-login-token exchange the first time IAM refuses.
type awsTransport struct {
	api         mwaaAPI
	environment string
	region      string
	// base is the HTTP client the fallback's requests ride, before the session
	// cookie jar is added to it.
	base *http.Client

	mu sync.Mutex
	// fallback is the web-login transport once it has been opened. Every
	// request after the first refusal goes straight to it: the AWS API's answer
	// will not change inside one command.
	fallback airflowapi.Transport
}

// Do carries one Airflow request through whichever way in is open.
//
// Request.Header is refused rather than dropped. The AWS API takes a path, a
// method, a query, and a body — there is nowhere to put a header — and the
// header a caller is most likely to set is Authorization, which the transport
// contract promises will override the transport's own credentials. It cannot
// here: the request is signed as the caller's AWS identity whatever they ask
// for. Silently sending it as somebody else is worse than saying no.
func (t *awsTransport) Do(ctx context.Context, req airflowapi.Request) (airflowapi.Response, error) {
	if fallback := t.heldFallback(); fallback != nil {
		return fallback.Do(ctx, req)
	}
	if len(req.Header) > 0 {
		return airflowapi.Response{}, fmt.Errorf("%w: MWAA's AWS API carries no request headers, so %s cannot be sent for %s. This request is signed as your AWS identity and nothing can override that. Reach this environment by URL if you need to set headers",
			airflowapi.ErrHeadersUnsupported, strings.Join(headerNames(req.Header), ", "), t.environment)
	}
	if req.Generation == airflowapi.GenerationNone {
		// A request below the API prefix — /health, /openapi.json — has no
		// server root to reach here: InvokeRestApi carries the versioned API
		// and nothing else. 404 is what the Transport contract asks an entrance
		// with no unversioned paths to answer, and the client reads it as
		// "not served" and moves on.
		return airflowapi.Response{StatusCode: http.StatusNotFound}, nil
	}
	input, err := t.invokeInput(req)
	if err != nil {
		return airflowapi.Response{}, err
	}
	out, err := t.api.InvokeRestApi(ctx, input)
	if err == nil {
		return restAPIResponse(out.RestApiStatusCode, http.StatusOK, out.RestApiResponse, nil)
	}
	// Airflow's own answer, wrapped: a 404 for a dag that is not there, a 403
	// from its role checks. The contract says a status is never an error, so
	// unwrap it back into the answer it started as.
	if resp, ok, rerr := wrappedAirflowAnswer(err); ok {
		return resp, rerr
	}
	if !isAccessDenied(err) {
		return airflowapi.Response{}, t.callFailure(err)
	}
	fallback, ferr := t.openFallback(ctx, err)
	if ferr != nil {
		return airflowapi.Response{}, ferr
	}
	return fallback.Do(ctx, req)
}

// callFailure names what the AWS API refused. A missing environment is the one
// worth spelling out: the name and the region come from two different places in
// the manifest, and "not found" without saying which two is a hunt.
func (t *awsTransport) callFailure(err error) error {
	var missing *mwaatypes.ResourceNotFoundException
	if errors.As(err, &missing) {
		return fmt.Errorf("no MWAA environment named %s in %s — check `environment` on this link and `region` under [tool.astro.targets.mwaa] in pyproject.toml (an environment in another region reads the same way)",
			t.environment, t.region)
	}
	return fmt.Errorf("call the MWAA API for %s in %s: %w", t.environment, t.region, err)
}

func (t *awsTransport) heldFallback() airflowapi.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fallback
}

// invokeInput turns an Airflow request into the AWS call that carries it.
// Request.Path arrives without a version prefix and goes over unchanged: this
// door picks the generation itself, which is exactly the case the Transport
// contract describes.
func (t *awsTransport) invokeInput(req airflowapi.Request) (*mwaa.InvokeRestApiInput, error) {
	method, err := restAPIMethod(req.Method)
	if err != nil {
		return nil, err
	}
	input := &mwaa.InvokeRestApiInput{
		Name:   aws.String(t.environment),
		Path:   aws.String(leadingSlash(req.Path)),
		Method: method,
	}
	if query := queryDocument(req.Query); query != nil {
		input.QueryParameters = document.NewLazyDocument(query)
	}
	if req.Body != nil {
		body, err := plainJSON(req.Body)
		if err != nil {
			return nil, err
		}
		input.Body = document.NewLazyDocument(body)
	}
	return input, nil
}

// plainJSON reduces a request body to the values a smithy document can carry:
// maps, slices, strings, numbers, booleans, and nil.
//
// The document encoder walks the Go value itself and refuses anything it does
// not recognize, which includes time.Time — and a time.Time is exactly what
// this package's own trigger call puts in a body. Encoding to JSON and back
// hands the encoder the shape the wire would have had anyway, so a type with a
// MarshalJSON of its own arrives as whatever it marshals to.
//
// Decoded without UseNumber on purpose: a json.Number is a string to the
// document encoder and would be sent quoted. The cost is that an integer past
// float64's exact range loses precision on the way out. Airflow request bodies
// carry ids, dates, and counts, none of which reach that size, and a response
// — where large numbers do turn up — never goes through here.
func plainJSON(body any) (any, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode the request body for MWAA: %w", err)
	}
	var plain any
	if err := json.Unmarshal(encoded, &plain); err != nil {
		return nil, fmt.Errorf("encode the request body for MWAA: %w", err)
	}
	return plain, nil
}

// restAPIMethods is the closed set of methods the AWS API carries, taken from
// the SDK so it cannot drift from what MWAA accepts, and spelled out once for
// the message that lists them.
var (
	restAPIMethods    = mwaatypes.RestApiMethodGet.Values()
	restAPIMethodList = restAPIMethodNames()
)

func restAPIMethodNames() string {
	names := make([]string, len(restAPIMethods))
	for i, method := range restAPIMethods {
		names[i] = string(method)
	}
	return strings.Join(names, ", ")
}

// restAPIMethod maps an HTTP method onto that set. A method outside it is
// named rather than sent, because the AWS API would refuse it with a
// validation error nobody could act on.
func restAPIMethod(method string) (mwaatypes.RestApiMethod, error) {
	if method == "" {
		return mwaatypes.RestApiMethodGet, nil
	}
	for _, known := range restAPIMethods {
		if strings.EqualFold(method, string(known)) {
			return known, nil
		}
	}
	return "", fmt.Errorf("MWAA's AWS API does not carry %s requests, only %s", method, restAPIMethodList)
}

// headerNames lists a header set for a message, sorted so the sentence reads
// the same twice.
func headerNames(header http.Header) []string {
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// leadingSlash makes a path absolute. Request.Path arrives however a caller
// spelled it and the AWS API takes only the absolute form.
func leadingSlash(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

// queryDocument turns a query string into the JSON object MWAA takes. A name
// with one value becomes that value and a name with several keeps its list,
// which is how Airflow's own API reads a repeated parameter.
func queryDocument(query url.Values) map[string]any {
	if len(query) == 0 {
		return nil
	}
	out := make(map[string]any, len(query))
	for name, values := range query {
		if len(values) == 1 {
			out[name] = values[0]
			continue
		}
		out[name] = values
	}
	return out
}

// restAPIResponse turns the AWS API's wrapper back into the Airflow answer it
// holds. The status is a pointer on every one of MWAA's shapes, so each caller
// says what an absent one means: a successful call that reported no status
// said 200, and a refusal that reported none is still a refusal — reading it
// as 200 would turn "denied" into "no results".
//
// fallbackBody stands in when the wrapper carries no document. On the success
// path there is nothing to stand in for and it is empty; on a refusal it is
// MWAA's own sentence, because an error with an empty body tells the reader
// only a number.
func restAPIResponse(status *int32, whenAbsent int, body document.Interface, fallbackBody []byte) (airflowapi.Response, error) {
	code := whenAbsent
	if status != nil {
		code = int(*status)
	}
	resp := airflowapi.Response{StatusCode: code, Header: http.Header{}}
	payload, err := documentJSON(body)
	if err != nil {
		return airflowapi.Response{}, err
	}
	if payload == nil {
		payload = fallbackBody
	}
	if payload == nil {
		return resp, nil
	}
	resp.Header.Set("Content-Type", "application/json")
	resp.Body = payload
	return resp, nil
}

// documentJSON re-encodes a smithy document as the JSON Airflow sent. The
// document holds the decoded response verbatim, numbers included, so this is a
// round trip rather than a reinterpretation.
//
// A document that is absent or literally null carries nothing. Null is dropped
// rather than passed on because a caller decoding it would get its zero value
// and no way to tell that from an answer of nothing, and "no payload" is the
// truer report. A document that will not re-encode is a failure of this
// translation, not an answer, so it travels as an error.
func documentJSON(body document.Interface) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	payload, err := body.MarshalSmithyDocument()
	if err != nil {
		return nil, fmt.Errorf("read the answer MWAA carried back: %w", err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		return nil, nil
	}
	return payload, nil
}

// wrappedAirflowAnswer unwraps the two exceptions that are not failures of the
// AWS call at all: Airflow answered, and MWAA reported its status as an error
// because that is how the AWS API models a non-2xx. The Transport contract
// says a status travels in the response, so these become responses.
func wrappedAirflowAnswer(err error) (airflowapi.Response, bool, error) {
	var client *mwaatypes.RestApiClientException
	if errors.As(err, &client) {
		resp, rerr := restAPIResponse(client.RestApiStatusCode, http.StatusBadRequest, client.RestApiResponse, detailBody(err))
		return resp, true, rerr
	}
	var server *mwaatypes.RestApiServerException
	if errors.As(err, &server) {
		resp, rerr := restAPIResponse(server.RestApiStatusCode, http.StatusBadGateway, server.RestApiResponse, detailBody(err))
		return resp, true, rerr
	}
	return airflowapi.Response{}, false, nil
}

// detailBody dresses the exception as the error shape Airflow would have sent,
// so a caller reading *StatusError.Detail finds an explanation in the place it
// always looks rather than a bare status and nothing else.
//
// It is the whole error rather than the exception's Message because the
// generated deserializer throws that field away: MWAA sends a "message" and the
// SDK's decoder has no case for it, so ErrorMessage() is always empty. What
// survives is the operation, the AWS status, and the exception's name, which at
// least says who refused.
func detailBody(err error) []byte {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return nil
	}
	payload, jerr := json.Marshal(map[string]string{"detail": message})
	if jerr != nil {
		return nil
	}
	return payload
}

// isAccessDenied reports the AWS API shutting the InvokeRestApi door: either
// the caller's IAM policy is missing airflow:InvokeRestAPI, or the Airflow
// role it maps to is not allowed to make the call. Both mean this door is
// closed and the other one may not be, which is the whole reason the fallback
// exists.
func isAccessDenied(err error) bool {
	var denied *mwaatypes.AccessDeniedException
	return errors.As(err, &denied)
}
