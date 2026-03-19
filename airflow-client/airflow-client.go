package airflowclient

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/sirupsen/logrus"
)

var errDecode = errors.New("failed to decode response from API")

// sleepFn is a variable so tests can replace it with a no-op to avoid real sleeps.
var sleepFn = time.Sleep

const (
	pageLimit    = 100
	maxRetries   = 10
	retryBackoff = time.Second
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
	if err != nil {
		return err
	}
	return nil
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
	if err != nil {
		return err
	}
	return nil
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
	if err != nil {
		return err
	}
	return nil
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

// isRetryable returns true for errors that warrant a retry: 5xx HTTP errors and
// connection-level errors (timeouts, resets, etc.). Context cancellation and
// deadline exceeded are not retried.
func isRetryable(err error) bool {
	if errors.Is(err, stdctx.Canceled) || errors.Is(err, stdctx.DeadlineExceeded) {
		return false
	}
	var httpErr *httputil.Error
	if errors.As(err, &httpErr) {
		return httpErr.Status >= http.StatusInternalServerError
	}
	// Non-httputil errors are connection-level failures.
	return true
}

func (c *HTTPClient) DoAirflowClient(doOpts *httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext() // get current context
	if err != nil {
		return nil, err
	}

	if cl.Token != "" {
		doOpts.Headers = map[string]string{
			"authorization": cl.Token,
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logrus.Warnf("Airflow API request failed (%v), retrying (attempt %d/%d)...", lastErr, attempt, maxRetries)
			sleepFn(retryBackoff)
		}

		response, err := c.Do(doOpts)
		if err != nil {
			if isRetryable(err) {
				lastErr = err
				continue
			}
			return nil, err
		}

		// Check the response status code, return error for non-2xx codes
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
		}

		decode := Response{}
		decodeErr := json.NewDecoder(response.Body).Decode(&decode)
		response.Body.Close()
		if decodeErr != nil {
			return nil, errDecode
		}

		return &decode, nil
	}

	return nil, lastErr
}
