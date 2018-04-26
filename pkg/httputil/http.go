package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"github.com/shurcooL/go/ctxhttp"
)

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
	Data      interface{}
	Context   context.Context
	Headers   map[string]string
	ForceJSON bool
}

// NewHTTPClient returns a new HTTP Client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		HTTPClient: &http.Client{},
	}
}

// Do executes the given HTTP request and returns the HTTP Response
func (c *HTTPClient) Do(method, path string, doOptions *DoOptions) (*http.Response, error) {
	var body io.Reader
	if doOptions.Data != nil || doOptions.ForceJSON {
		buf, err := json.Marshal(doOptions.Data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(buf)
	}

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	if doOptions.Data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range doOptions.Headers {
		req.Header.Set(k, v)
	}

	ctx := doOptions.Context
	if ctx == nil {
		ctx = context.Background()
	}

	resp, err := ctxhttp.Do(ctx, c.HTTPClient, req)
	if err != nil {
		return nil, errors.Wrap(chooseError(ctx, err), "HTTP DO Failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, newError(resp)
	}
	return resp, nil
}

// if error in context, return that instead of generic http error
func chooseError(ctx context.Context, err error) error {
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
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &Error{Status: resp.StatusCode, Message: fmt.Sprintf("cannot read body, err: %v", err)}
	}
	return &Error{Status: resp.StatusCode, Message: string(data)}
}

// Error implemented to match Error interface
func (e *Error) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.Status, e.Message)
}
