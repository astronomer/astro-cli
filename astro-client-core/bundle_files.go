package astrocore

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// BundleFilesStreamClient provides streaming HTTP operations for bundle file
// uploads and downloads. The generated oapi-codegen client does not support
// request bodies for upload endpoints or streaming responses for download
// endpoints, so these methods are implemented manually.
type BundleFilesStreamClient interface {
	UploadBundleArchive(ctx httpContext.Context, orgID, deploymentID string, body io.Reader, contentLength int64, overwrite bool, targetPath string) error
	DownloadBundleArchive(ctx httpContext.Context, orgID, deploymentID string) (io.ReadCloser, error)
	UploadBundleFile(ctx httpContext.Context, orgID, deploymentID, filePath string, body io.Reader, contentLength int64) error
	DownloadBundleFile(ctx httpContext.Context, orgID, deploymentID, filePath string) (io.ReadCloser, error)
}

type bundleFilesStreamClient struct {
	httpClient *http.Client
}

func NewBundleFilesStreamClient(httpClient *http.Client) BundleFilesStreamClient {
	return &bundleFilesStreamClient{httpClient: httpClient}
}

func (c *bundleFilesStreamClient) UploadBundleArchive(ctx httpContext.Context, orgID, deploymentID string, body io.Reader, contentLength int64, overwrite bool, targetPath string) error {
	path := fmt.Sprintf("/organizations/%s/deployments/%s/bundle/archive", url.PathEscape(orgID), url.PathEscape(deploymentID))

	params := url.Values{}
	if overwrite {
		params.Set("overwrite", "true")
	}
	if targetPath != "" {
		params.Set("targetPath", targetPath)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkResponse(resp)
}

func (c *bundleFilesStreamClient) DownloadBundleArchive(ctx httpContext.Context, orgID, deploymentID string) (io.ReadCloser, error) {
	path := fmt.Sprintf("/organizations/%s/deployments/%s/bundle/archive", url.PathEscape(orgID), url.PathEscape(deploymentID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := checkResponse(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	return resp.Body, nil
}

func (c *bundleFilesStreamClient) UploadBundleFile(ctx httpContext.Context, orgID, deploymentID, filePath string, body io.Reader, contentLength int64) error {
	path := fmt.Sprintf("/organizations/%s/deployments/%s/bundle/files/upload/%s", url.PathEscape(orgID), url.PathEscape(deploymentID), filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkResponse(resp)
}

func (c *bundleFilesStreamClient) DownloadBundleFile(ctx httpContext.Context, orgID, deploymentID, filePath string) (io.ReadCloser, error) {
	path := fmt.Sprintf("/organizations/%s/deployments/%s/bundle/files/download/%s", url.PathEscape(orgID), url.PathEscape(deploymentID), filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := checkResponse(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	return resp.Body, nil
}

// doRequest applies the CoreRequestEditor (for auth + base URL) and executes the request.
func (c *bundleFilesStreamClient) doRequest(ctx httpContext.Context, req *http.Request) (*http.Response, error) {
	if err := CoreRequestEditor(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrorRequest, err)
	}
	return resp, nil
}

// checkResponse returns an error if the HTTP status code indicates failure.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	decode := Error{}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode); err != nil {
		return fmt.Errorf("%w, status %d", ErrorRequest, resp.StatusCode)
	}
	return fmt.Errorf("%s", decode.Message) //nolint:goerr113
}
