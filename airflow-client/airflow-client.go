package airflowclient

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var errDecode = errors.New("failed to decode response from API")

// isConflict reports whether err is an HTTP 409 Conflict.
func isConflict(err error) bool {
	var httpErr *httputil.Error
	if errors.As(err, &httpErr) {
		return httpErr.Status == http.StatusConflict
	}
	return false
}

// isReadMethod reports whether method is a safe, read-only HTTP method.
func isReadMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

const (
	pageLimit         = 100
	readMaxRetries    = 10
	readRetryBackoff  = time.Second
	writeMaxRetries   = 3
	writeRetryBackoff = 5 * time.Second
)

type Client interface {
	// connections
	GetConnections(airflowURL string) (Response, error)
	CreateConnection(airflowURL string, conn *Connection) error
	UpdateConnection(airflowURL string, conn *Connection) error
	// variables
	GetVariables(airflowURL string) (Response, error)
	CreateVariable(airflowURL string, variable Variable) error
	UpdateVariable(airflowURL string, variable Variable) error
	// pools
	GetPools(airflowURL string) (Response, error)
	CreatePool(airflowURL string, pool Pool) error
	UpdatePool(airflowURL string, pool Pool) error
}

// Client containers the logger and HTTPClient used to communicate with the Astronomer API
type HTTPClient struct {
	*httputil.HTTPClient
}

// NewAstroClient returns a new Client with the logger and HTTP client setup.
func NewAirflowClient(c *httputil.HTTPClient) *HTTPClient {
	return &HTTPClient{
		c,
	}
}

// fetchAllPages fetches paginated results from an Airflow REST endpoint, accumulating
// all items across pages. It stops on the first page with fewer than pageLimit entries, or when
// the offset exceeds the response's TotalEntries value (if present).
func fetchAllPages[T any](c *HTTPClient, airflowURL, resource string, extract func(Response) []T) ([]T, error) {
	var all []T
	offset := 0
	for {
		doOpts := &httputil.DoOptions{
			Path:   fmt.Sprintf("https://%s/%s?limit=%d&offset=%d", airflowURL, resource, pageLimit, offset),
			Method: http.MethodGet,
		}
		page, err := c.DoAirflowClient(doOpts)
		if err != nil {
			return nil, err
		}
		items := extract(*page)
		all = append(all, items...)
		offset += len(items)
		if len(items) < pageLimit || (page.TotalEntries > 0 && offset >= page.TotalEntries) {
			break
		}
	}
	return all, nil
}

func (c *HTTPClient) GetConnections(airflowURL string) (Response, error) {
	conns, err := fetchAllPages(c, airflowURL, "connections", func(r Response) []Connection { return r.Connections })
	if err != nil {
		return Response{}, err
	}
	return Response{Connections: conns}, nil
}

func (c *HTTPClient) CreateConnection(airflowURL string, conn *Connection) error {
	// Convert the connection struct to JSON bytes
	connJSON, err := json.Marshal(&conn)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/connections",
		Method: http.MethodPost,
		Data:   connJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if isConflict(err) {
		// already exists: reconcile to the desired state
		return c.UpdateConnection(airflowURL, conn)
	}
	return err
}

func (c *HTTPClient) UpdateConnection(airflowURL string, conn *Connection) error {
	// Convert the connection struct to JSON bytes
	connJSON, err := json.Marshal(&conn)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   fmt.Sprintf("https://%s/connections/%s", airflowURL, conn.ConnID),
		Method: http.MethodPatch,
		Data:   connJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) GetVariables(airflowURL string) (Response, error) {
	vars, err := fetchAllPages(c, airflowURL, "variables", func(r Response) []Variable { return r.Variables })
	if err != nil {
		return Response{}, err
	}
	return Response{Variables: vars}, nil
}

func (c *HTTPClient) CreateVariable(airflowURL string, variable Variable) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(variable)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/variables",
		Method: http.MethodPost,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if isConflict(err) {
		// already exists: reconcile to the desired state
		return c.UpdateVariable(airflowURL, variable)
	}
	return err
}

func (c *HTTPClient) UpdateVariable(airflowURL string, variable Variable) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(variable)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   fmt.Sprintf("https://%s/variables/%s", airflowURL, variable.Key),
		Method: http.MethodPatch,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) GetPools(airflowURL string) (Response, error) {
	pools, err := fetchAllPages(c, airflowURL, "pools", func(r Response) []Pool { return r.Pools })
	if err != nil {
		return Response{}, err
	}
	return Response{Pools: pools}, nil
}

func (c *HTTPClient) CreatePool(airflowURL string, pool Pool) error {
	// Convert the connection struct to JSON bytes
	varJSON, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	doOpts := &httputil.DoOptions{
		Path:   "https://" + airflowURL + "/pools",
		Method: http.MethodPost,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if isConflict(err) {
		// already exists: reconcile to the desired state
		return c.UpdatePool(airflowURL, pool)
	}
	return err
}

func (c *HTTPClient) UpdatePool(airflowURL string, pool Pool) error {
	path := fmt.Sprintf("https://%s/pools/%s", airflowURL, pool.Name)

	// default pool does not allow updating other fields, such as description
	if pool.Name == "default_pool" {
		path += "?update_mask=slots&update_mask=include_deferred"
	}

	varJSON, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	doOpts := &httputil.DoOptions{
		Path:   path,
		Method: http.MethodPatch,
		Data:   varJSON,
	}

	_, err = c.DoAirflowClient(doOpts)
	if err != nil {
		return err
	}

	return nil
}

// checkRetryPolicy retries read methods with the default policy and writes only on transient statuses.
func checkRetryPolicy(method string) retryablehttp.CheckRetry {
	return func(ctx stdctx.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			if errors.Is(err, stdctx.Canceled) || errors.Is(err, stdctx.DeadlineExceeded) {
				return false, err
			}
		}
		if !isReadMethod(method) {
			if err != nil {
				return false, err
			}
			return isRetryableWriteStatus(resp), nil
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}
}

func isRetryableWriteStatus(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func (c *HTTPClient) DoAirflowClient(doOpts *httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	if cl.Token != "" {
		doOpts.Headers = map[string]string{
			"authorization": cl.Token,
		}
	}

	req, err := retryablehttp.NewRequest(doOpts.Method, doOpts.Path, doOpts.Data)
	if err != nil {
		return nil, err
	}
	if len(doOpts.Data) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range doOpts.Headers {
		req.Header.Set(k, v)
	}

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient = c.HTTPClient.HTTPClient
	retryClient.RetryMax = readMaxRetries
	retryClient.RetryWaitMin = readRetryBackoff
	retryClient.RetryWaitMax = readRetryBackoff
	if !isReadMethod(doOpts.Method) {
		retryClient.RetryMax = writeMaxRetries
		retryClient.RetryWaitMin = writeRetryBackoff
		retryClient.RetryWaitMax = writeRetryBackoff
	}
	retryClient.CheckRetry = checkRetryPolicy(doOpts.Method)
	retryClient.ErrorHandler = retryablehttp.PassthroughErrorHandler
	retryClient.Logger = nil // suppress retryablehttp's default stderr logging on retries

	resp, err := retryClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return nil, &httputil.Error{Status: resp.StatusCode, Message: string(data)}
	}

	decode := Response{}
	if err := json.NewDecoder(resp.Body).Decode(&decode); err != nil {
		return nil, errDecode
	}
	return &decode, nil
}
