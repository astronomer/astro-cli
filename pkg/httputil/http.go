package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/pkg/errors"
	"golang.org/x/net/context/ctxhttp"
)

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
	Context context.Context
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

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
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
		doCtx = context.Background()
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
	printutil.LogFatal(err)

	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	resp, err := client.Get(sourceURL) //nolint
	defer resp.Body.Close()
	printutil.LogFatal(err)

	err = fileutil.WriteToFile(path, resp.Body)
	printutil.LogFatal(err)

	defer file.Close()
	logrus.Infof("Downloaded %s from %s", path, sourceURL)
}

func RequestAndGetJSONBody(route string) map[string]interface{} {
	res, err := http.Get(route) //nolint
	defer res.Body.Close()
	printutil.LogFatal(err)

	body, err := io.ReadAll(res.Body)
	printutil.LogFatal(err)
	if res.StatusCode > LastSuccessfulHTTPResponseCode {
		logrus.Fatalf("Response failed with status code: %d and\nbody: %s\n", res.StatusCode, body)
	}

	var bodyJSON map[string]interface{}
	err = json.Unmarshal(body, &bodyJSON)
	printutil.LogFatal(err)
	logrus.Debugf("%s - GET %s %s", res.Status, route, string(body))
	return bodyJSON
}
