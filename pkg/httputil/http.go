package httputil

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/pkg/logger"

	"github.com/astronomer/astro-cli/pkg/fileutil"

	"github.com/pkg/errors"
	"golang.org/x/net/context/ctxhttp"
)

var ErrorBaseURL = errors.New("invalid baseurl")

const LastSuccessfulHTTPResponseCode = 299

// HTTPClient returns an HTTP Client struct that can execute HTTP requests
type HTTPClient struct {
	HTTPClient *http.Client
}

// HTTPResponse houses respnse object
type HTTPResponse struct {
	Raw  *http.Response
	Body string
}

// DoOptions are options passed to the HTTPClient.Do function
type DoOptions struct {
	Data    []byte
	Context httpContext.Context
	Headers map[string]string
	Method  string
	Path    string
}

// NewHTTPClient returns a new HTTP Client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		HTTPClient: &http.Client{},
	}
}

// Do executes the given HTTP request and returns the HTTP Response
func (c *HTTPClient) Do(doOptions *DoOptions) (*http.Response, error) {
	var body io.Reader
	if len(doOptions.Data) > 0 {
		body = bytes.NewBuffer(doOptions.Data)
	}

	ctx := httpContext.Background()
	ctx, cancel := httpContext.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, doOptions.Method, doOptions.Path, body)
	if err != nil {
		return nil, err
	}
	if len(doOptions.Data) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range doOptions.Headers {
		req.Header.Set(k, v)
	}

	doCtx := doOptions.Context
	if doCtx == nil {
		doCtx = httpContext.Background()
	}

	resp, err := ctxhttp.Do(doCtx, c.HTTPClient, req)
	if err != nil {
		return nil, errors.Wrap(chooseError(doCtx, err), "HTTP DO Failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, newError(resp)
	}
	return resp, nil
}

// if error in context, return that instead of generic http error
func chooseError(ctx httpContext.Context, err error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return err
	}
}

// Error is a custom HTTP error structure
type Error struct {
	Status  int
	Message string
}

func newError(resp *http.Response) *Error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Error{Status: resp.StatusCode, Message: fmt.Sprintf("cannot read body, err: %v", err)}
	}
	return &Error{Status: resp.StatusCode, Message: string(data)}
}

// Error implemented to match Error interface
func (e *Error) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.Status, e.Message)
}

func DownloadResponseToFile(sourceURL, path string) {
	file, err := fileutil.CreateFile(path)
	if err != nil {
		logger.Logger.Fatal(err)
	}

	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	resp, err := client.Get(sourceURL) //nolint
	if err != nil {
		logger.Logger.Fatal(err)
	}
	defer resp.Body.Close()

	err = fileutil.WriteToFile(path, resp.Body)
	if err != nil {
		logger.Logger.Fatal(err)
	}
	defer file.Close()
	logger.Logger.Infof("Downloaded %s from %s", path, sourceURL)
}

func RequestAndGetJSONBody(route string) map[string]interface{} {
	res, err := http.Get(route) //nolint
	if err != nil {
		logger.Logger.Fatal(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Logger.Fatal(err)
	}
	if res.StatusCode > LastSuccessfulHTTPResponseCode {
		logger.Logger.Fatalf("Response failed with status code: %d and\nbody: %s\n", res.StatusCode, body)
	}

	var bodyJSON map[string]interface{}
	err = json.Unmarshal(body, &bodyJSON)
	if err != nil {
		logger.Logger.Fatal(err)
	}
	logger.Logger.Debugf("%s - GET %s %s", res.Status, route, string(body))
	return bodyJSON
}
