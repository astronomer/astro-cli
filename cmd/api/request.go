package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/fatih/color"
)

const (
	defaultTimeout     = 30 * time.Second
	httpStatusError    = 400
	httpStatusRedirect = 300
	maxPaginationPages = 100
	minTokenLength     = 8
)

// SilentError is returned when the error has already been presented to the
// user (e.g. an HTTP error body was printed) and cobra should exit non-zero
// without printing the error text again.
type SilentError struct {
	StatusCode int
}

func (e *SilentError) Error() string {
	return fmt.Sprintf("API request failed with status %d", e.StatusCode)
}

// isConnectionError reports whether err indicates a network-level connection
// failure (connection refused, DNS lookup failure, timeout, etc.) as opposed
// to an HTTP-level error.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// isLocalhostURL checks if a URL string points to localhost, 127.0.0.1, or [::1].
func isLocalhostURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// RequestOptions holds the common options shared by all API commands (cloud, airflow, etc.).
type RequestOptions struct {
	Out    io.Writer
	ErrOut io.Writer // Writer for warnings/diagnostics (defaults to os.Stderr)

	// Request options
	RequestMethod       string
	RequestMethodPassed bool
	RequestPath         string
	RequestInputFile    string
	MagicFields         []string
	RawFields           []string
	RequestHeaders      []string
	PathParams          []string

	// Output options
	ShowResponseHeaders bool
	Paginate            bool
	Slurp               bool
	Silent              bool
	Template            string
	FilterOutput        string
	Verbose             bool

	// Other options
	GenerateCurl bool

	// Internal
	specCache  *openapi.Cache
	HTTPClient *http.Client // HTTP client for making requests (defaults to http.DefaultClient)
}

// httpResult captures the essential parts of an HTTP response after the body
// has been fully read and the connection released. This avoids returning a
// half-closed *http.Response from doRequest.
type httpResult struct {
	StatusCode int
	Status     string
	Proto      string
	Header     http.Header
	Body       []byte
}

// GetErrOut returns the error output writer, defaulting to os.Stderr.
func (o *RequestOptions) GetErrOut() io.Writer {
	if o.ErrOut != nil {
		return o.ErrOut
	}
	return os.Stderr
}

// GetHTTPClient returns the HTTP client, defaulting to http.DefaultClient.
func (o *RequestOptions) GetHTTPClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

// setCustomHeaders parses and applies custom headers to the request.
// Each header must be in "key:value" format.
func setCustomHeaders(req *http.Request, headers []string) error {
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid header format %q, expected key:value", h)
		}
		req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	return nil
}

// doRequest builds and executes an HTTP request, returning an httpResult with
// the status, headers, and fully-read body. The underlying connection is released
// before this function returns.
func doRequest(opts *RequestOptions, method, requestURL, token string, body io.Reader, bodyBytes []byte) (*httpResult, error) {
	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set standard headers
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil && body != http.NoBody {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add custom headers (consistent validation in both paths)
	if err := setCustomHeaders(req, opts.RequestHeaders); err != nil {
		return nil, err
	}

	// Verbose: print request
	if opts.Verbose {
		printRequest(opts.Out, req, bodyBytes)
	}

	// Execute request
	resp, err := opts.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	// Read the entire body and close immediately so the connection is released.
	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("reading response: %w", readErr)
	}

	result := &httpResult{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Proto:      resp.Proto,
		Header:     resp.Header,
		Body:       respBody,
	}

	// Verbose: print response headers
	if opts.Verbose {
		printResponseHeaders(opts.Out, resp)
	}

	return result, nil
}

// executeRequest builds and executes the HTTP request.
func executeRequest(opts *RequestOptions, method, requestURL, token string, params map[string]interface{}) error {
	// Handle pagination
	if opts.Paginate && strings.EqualFold(method, http.MethodGet) {
		return executePaginatedRequest(opts, method, requestURL, token, params)
	}

	return executeSingleRequest(opts, method, requestURL, token, params)
}

// executeSingleRequest executes a single HTTP request.
func executeSingleRequest(opts *RequestOptions, method, requestURL, token string, params map[string]interface{}) error {
	var body io.Reader
	var bodyBytes []byte

	// Handle request body
	if opts.RequestInputFile != "" {
		// Read body from file
		var err error
		bodyBytes, err = readInputFile(opts.RequestInputFile)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bodyBytes)
		// If params provided, add them as query string
		if len(params) > 0 {
			requestURL = addQueryParams(requestURL, params)
		}
	} else if len(params) > 0 {
		if strings.EqualFold(method, http.MethodGet) {
			// For GET requests, add params as query string
			requestURL = addQueryParams(requestURL, params)
		} else {
			// For other methods, JSON encode params as body
			var err error
			bodyBytes, err = json.Marshal(params)
			if err != nil {
				return fmt.Errorf("marshaling request body: %w", err)
			}
			body = bytes.NewReader(bodyBytes)
		}
	}

	result, err := doRequest(opts, method, requestURL, token, body, bodyBytes)
	if err != nil {
		return err
	}

	// Print response headers if requested (skip if verbose already printed them)
	if opts.ShowResponseHeaders && !opts.Verbose {
		fmt.Fprintf(opts.Out, "%s %s\n", result.Proto, result.Status)
		printHeaders(opts.Out, result.Header, isColorEnabled(opts.Out))
		fmt.Fprintln(opts.Out)
	}

	// Handle error responses: print the body, then return a SilentError so cobra
	// exits non-zero without printing its own "Error: ..." line (SilenceErrors is
	// set on the parent api command).
	if result.StatusCode >= httpStatusError {
		if len(result.Body) > 0 {
			_ = writeColorizedJSON(opts.Out, result.Body, isColorEnabled(opts.Out), "  ")
		}
		return &SilentError{StatusCode: result.StatusCode}
	}

	return outputResponseBody(opts, result.Body)
}

// outputResponseBody handles silent mode, empty responses, and output processing.
func outputResponseBody(opts *RequestOptions, respBody []byte) error {
	// Handle silent mode
	if opts.Silent {
		return nil
	}

	// Handle empty response
	if len(respBody) == 0 {
		return nil
	}

	// Process output
	outputOpts := OutputOptions{
		FilterOutput: opts.FilterOutput,
		Template:     opts.Template,
		ColorEnabled: isColorEnabled(opts.Out),
		Indent:       "  ",
	}

	return processOutput(respBody, opts.Out, outputOpts)
}

// executePaginatedRequest handles paginated GET requests.
func executePaginatedRequest(opts *RequestOptions, method, requestURL, token string, params map[string]interface{}) error {
	var allPages []json.RawMessage
	currentOffset := 0
	pageNum := 0

	for {
		pageNum++
		// Build URL with current offset
		pageURL := addOffsetToURL(requestURL, currentOffset)

		// Make request
		respBody, totalCount, limit, err := fetchPage(opts, method, pageURL, token, params)
		if err != nil {
			return err
		}

		if len(respBody) == 0 {
			break
		}

		if opts.Slurp {
			// Collect pages for slurping
			allPages = append(allPages, respBody)
		} else {
			// Print each page immediately
			outputOpts := OutputOptions{
				FilterOutput: opts.FilterOutput,
				Template:     opts.Template,
				ColorEnabled: isColorEnabled(opts.Out),
				Indent:       "  ",
			}
			if err := processOutput(respBody, opts.Out, outputOpts); err != nil {
				return err
			}
		}

		// Check if there are more pages
		if totalCount <= 0 {
			break
		}
		if limit <= 0 {
			fmt.Fprintf(opts.GetErrOut(), "\nWarning: server returned totalCount=%d but no limit; stopping pagination\n", totalCount)
			break
		}
		currentOffset += limit
		if currentOffset >= totalCount {
			break
		}

		// Safety limit to prevent infinite loops
		if pageNum >= maxPaginationPages {
			fmt.Fprintf(opts.GetErrOut(), "\nWarning: reached maximum page limit (100)\n")
			break
		}
	}

	// If slurping, combine all pages and output
	if opts.Slurp && len(allPages) > 0 {
		combined, err := combinePages(allPages)
		if err != nil {
			return fmt.Errorf("combining pages: %w", err)
		}
		outputOpts := OutputOptions{
			FilterOutput: opts.FilterOutput,
			Template:     opts.Template,
			ColorEnabled: isColorEnabled(opts.Out),
			Indent:       "  ",
		}
		return processOutput(combined, opts.Out, outputOpts)
	}

	return nil
}

// fetchPage fetches a single page and returns the body, totalCount, and limit.
func fetchPage(opts *RequestOptions, method, requestURL, token string, params map[string]interface{}) (body []byte, totalCount, limit int, err error) {
	// Add params as query string for GET
	if len(params) > 0 {
		requestURL = addQueryParams(requestURL, params)
	}

	result, err := doRequest(opts, method, requestURL, token, http.NoBody, nil)
	if err != nil {
		return nil, 0, 0, err
	}

	// Handle error responses: print the body for diagnostics, then return a
	// SilentError to match the executeSingleRequest behavior.
	if result.StatusCode >= httpStatusError {
		if len(result.Body) > 0 {
			_ = writeColorizedJSON(opts.Out, result.Body, isColorEnabled(opts.Out), "  ")
		}
		return nil, 0, 0, &SilentError{StatusCode: result.StatusCode}
	}

	// Parse response to get pagination info
	var pageInfo struct {
		TotalCount int `json:"totalCount"`
		Offset     int `json:"offset"`
		Limit      int `json:"limit"`
	}
	if err := json.Unmarshal(result.Body, &pageInfo); err == nil {
		return result.Body, pageInfo.TotalCount, pageInfo.Limit, nil
	}

	// No pagination info found
	return result.Body, 0, 0, nil
}

// addOffsetToURL adds or updates the offset query parameter.
func addOffsetToURL(requestURL string, offset int) string {
	if offset == 0 {
		return requestURL
	}

	u, err := url.Parse(requestURL)
	if err != nil {
		return requestURL
	}

	q := u.Query()
	q.Set("offset", fmt.Sprintf("%d", offset))
	u.RawQuery = q.Encode()
	return u.String()
}

// combinePages combines multiple paginated responses into a single array.
func combinePages(pages []json.RawMessage) ([]byte, error) {
	if len(pages) == 0 {
		return []byte("[]"), nil
	}

	// Try to find the main array field in the responses (e.g., "organizations", "deployments")
	var firstPage map[string]interface{}
	if err := json.Unmarshal(pages[0], &firstPage); err != nil {
		// If not an object, just return array of pages
		result, err := json.Marshal(pages)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	// Find the array field (skip pagination fields).
	// Sort keys so the selection is deterministic when multiple array fields exist.
	keys := make([]string, 0, len(firstPage))
	for key := range firstPage {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var arrayField string
	for _, key := range keys {
		if key == "totalCount" || key == "offset" || key == "limit" {
			continue
		}
		if _, ok := firstPage[key].([]interface{}); ok {
			arrayField = key
			break
		}
	}

	if arrayField == "" {
		// No array field found, return array of pages
		result, err := json.Marshal(pages)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	// Combine all items from the array field
	var allItems []interface{}
	for i, page := range pages {
		var pageData map[string]interface{}
		if err := json.Unmarshal(page, &pageData); err != nil {
			return nil, fmt.Errorf("unmarshaling page %d: %w", i+1, err)
		}
		if items, ok := pageData[arrayField].([]interface{}); ok {
			allItems = append(allItems, items...)
		}
	}

	// Return combined result with just the array
	result := map[string]interface{}{
		arrayField: allItems,
	}
	return json.Marshal(result)
}

// readInputFile reads the request body from a file or stdin.
func readInputFile(filename string) ([]byte, error) {
	if filename == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(filename)
}

// addQueryParams adds params to the URL as query string.
func addQueryParams(requestURL string, params map[string]interface{}) string {
	if len(params) == 0 {
		return requestURL
	}

	u, err := url.Parse(requestURL)
	if err != nil {
		return requestURL
	}

	q := u.Query()
	for key, value := range params {
		addQueryParam(q, key, value)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// addQueryParam recursively adds a parameter to the query.
func addQueryParam(q url.Values, key string, value interface{}) {
	switch v := value.(type) {
	case string:
		q.Add(key, v)
	case int:
		q.Add(key, fmt.Sprintf("%d", v))
	case bool:
		q.Add(key, fmt.Sprintf("%v", v))
	case nil:
		q.Add(key, "")
	case []interface{}:
		for _, item := range v {
			addQueryParam(q, key+"[]", item)
		}
	case map[string]interface{}:
		for subkey, subvalue := range v {
			addQueryParam(q, key+"["+subkey+"]", subvalue)
		}
	default:
		q.Add(key, fmt.Sprintf("%v", v))
	}
}

// shellQuote wraps a string in single quotes for safe shell interpolation,
// escaping any embedded single quotes with the end-escape-reopen idiom.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// generateCurl generates a curl command for the request.
func generateCurl(out io.Writer, method, requestURL, token string, headers []string, params map[string]interface{}, inputFile string) error {
	method = strings.ToUpper(method)

	parts := make([]string, 0, 10+len(headers)*2)
	parts = append(parts, "curl")

	// Method
	if method != http.MethodGet {
		parts = append(parts, "-X", method)
	}

	// URL (with query params for GET, or when input file is used)
	if method == http.MethodGet && len(params) > 0 {
		requestURL = addQueryParams(requestURL, params)
	} else if inputFile != "" && len(params) > 0 {
		// When using input file, add params as query string
		requestURL = addQueryParams(requestURL, params)
	}
	// URL and standard headers
	parts = append(parts, shellQuote(requestURL))
	if token != "" {
		parts = append(parts, "-H", shellQuote("Authorization: "+token))
	}
	parts = append(parts, "-H", shellQuote("Accept: application/json"))

	// Custom headers
	for _, h := range headers {
		parts = append(parts, "-H", shellQuote(h))
	}

	// Body for non-GET requests
	if method != http.MethodGet {
		if inputFile != "" {
			// Read body from input file
			bodyBytes, err := readInputFile(inputFile)
			if err != nil {
				return fmt.Errorf("reading input file: %w", err)
			}
			parts = append(parts, "-H", shellQuote("Content-Type: application/json"), "-d", shellQuote(string(bodyBytes)))
		} else if len(params) > 0 {
			bodyBytes, err := json.Marshal(params)
			if err != nil {
				return fmt.Errorf("marshaling request body: %w", err)
			}
			parts = append(parts, "-H", shellQuote("Content-Type: application/json"), "-d", shellQuote(string(bodyBytes)))
		}
	}

	fmt.Fprintln(out, strings.Join(parts, " \\\n  "))
	return nil
}

// printRequest prints the request details for verbose mode.
func printRequest(out io.Writer, req *http.Request, body []byte) {
	fmt.Fprintf(out, "%s %s %s\n", color.CyanString(">"), color.GreenString(req.Method), req.URL.String())
	fmt.Fprintf(out, "%s Host: %s\n", color.CyanString(">"), req.Host)

	// Print headers
	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// Mask authorization header value
		val := strings.Join(req.Header[k], ", ")
		if strings.EqualFold(k, "authorization") {
			val = maskToken(val)
		}
		fmt.Fprintf(out, "%s %s: %s\n", color.CyanString(">"), k, val)
	}

	fmt.Fprintln(out, color.CyanString(">"))

	// Print body if present
	if len(body) > 0 {
		var prettyBody bytes.Buffer
		if err := json.Indent(&prettyBody, body, "", "  "); err == nil {
			for _, line := range strings.Split(prettyBody.String(), "\n") {
				fmt.Fprintf(out, "%s %s\n", color.CyanString(">"), line)
			}
		}
	}
	fmt.Fprintln(out)
}

// printResponseHeaders prints response headers for verbose mode.
func printResponseHeaders(out io.Writer, resp *http.Response) {
	statusColor := color.GreenString
	if resp.StatusCode >= httpStatusError {
		statusColor = color.RedString
	} else if resp.StatusCode >= httpStatusRedirect {
		statusColor = color.YellowString
	}

	fmt.Fprintf(out, "%s %s\n", color.CyanString("<"), statusColor("%s %s", resp.Proto, resp.Status))

	// Print headers
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(out, "%s %s: %s\n", color.CyanString("<"), k, strings.Join(resp.Header[k], ", "))
	}
	fmt.Fprintln(out, color.CyanString("<"))
	fmt.Fprintln(out)
}

// printHeaders prints HTTP headers.
func printHeaders(w io.Writer, headers http.Header, colorize bool) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		if name == "Status" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if colorize {
			fmt.Fprintf(w, "%s: %s\n", newColor(color.Bold, color.FgBlue).Sprint(name), strings.Join(headers[name], ", "))
		} else {
			fmt.Fprintf(w, "%s: %s\n", name, strings.Join(headers[name], ", "))
		}
	}
}

// maskToken masks most of a token for display.
func maskToken(token string) string {
	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	if len(token) <= minTokenLength {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
