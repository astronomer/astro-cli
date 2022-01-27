package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/shurcooL/go/ctxhttp"
	log "github.com/sirupsen/logrus"
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

	req, err := http.NewRequest(method, path, body) //nolint:noctx
	// req = req.WithContext(context.TODO())
	if err != nil {
		return nil, err
	}
	if doOptions.Data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range doOptions.Headers {
		req.Header.Set(k, v)
	}
	debug(httputil.DumpRequest(req, true))
	ctx := doOptions.Context
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()
	resp, err := ctxhttp.Do(ctx, c.HTTPClient, req)
	log.Debugf("Total time %v", time.Since(start))
	if err != nil {
		return nil, fmt.Errorf("HTTP DO Failed: %w", chooseError(ctx, err))
	}
	debug(httputil.DumpResponse(resp, true))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, newError(resp)
	}
	return resp, nil
}

func debug(data []byte, err error) {
	if err == nil {
		log.Debugf("%s\n\n", data)
	} else {
		log.Fatalf("%s\n\n", err)
	}
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
