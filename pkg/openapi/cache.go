package openapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/astronomer/astro-cli/config"
)

const (
	// SpecURL is the URL to fetch the Astro Cloud API OpenAPI specification.
	SpecURL = "https://api.astronomer.io/spec/v1.0"
	// CloudCacheFileName is the name of the cache file for the Astro Cloud API.
	CloudCacheFileName = "openapi-cache.json"
	// AirflowCacheFileNameTemplate is the template for version-specific Airflow cache files.
	AirflowCacheFileNameTemplate = "openapi-airflow-%s-cache.json"
	// CacheTTL is how long the cache is valid.
	CacheTTL = 24 * time.Hour
	// FetchTimeout is the timeout for fetching the spec.
	FetchTimeout = 30 * time.Second
	// dirPermissions is the permission mode for created directories.
	dirPermissions = 0o755
	// filePermissions is the permission mode for the cache file.
	filePermissions = 0o600
)

// AirflowCacheFileNameForVersion returns the cache file name for a specific Airflow version.
// The version is normalized to strip build metadata (e.g., "3.1.7+astro.1" -> "3.1.7").
func AirflowCacheFileNameForVersion(version string) string {
	normalizedVersion := NormalizeAirflowVersion(version)
	return fmt.Sprintf(AirflowCacheFileNameTemplate, normalizedVersion)
}

// CloudCacheFileNameForDomain returns the cache file name for a specific domain.
// For the default domain (astronomer.io), it returns the standard cache file name
// for backward compatibility. For other domains, a domain-qualified name is used
// to avoid cache collisions between environments.
func CloudCacheFileNameForDomain(domain string) string {
	if domain == "" || domain == "astronomer.io" {
		return CloudCacheFileName
	}
	normalized := strings.ReplaceAll(domain, ".", "_")
	return fmt.Sprintf("openapi-cache-%s.json", normalized)
}

// CachedSpec wraps raw spec data with metadata for caching.
// RawSpec is stored as []byte which json.Marshal encodes as base64.
type CachedSpec struct {
	RawSpec   []byte    `json:"rawSpec"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// Cache manages fetching and caching of an OpenAPI specification.
type Cache struct {
	specURL     string
	cachePath   string
	stripPrefix string // optional prefix to strip from endpoint paths (e.g. "/api/v2")
	authToken   string // optional auth token for fetching specs
	httpClient  *http.Client
	doc         *v3high.Document
	v2doc       *v2high.Swagger // non-nil if spec is Swagger 2.0
	rawSpec     []byte          // raw spec bytes for cache serialization
	fetchedAt   time.Time
}

// getHTTPClient returns the configured HTTP client, defaulting to http.DefaultClient.
func (c *Cache) getHTTPClient() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

// SetHTTPClient configures the HTTP client used to fetch specs.
func (c *Cache) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// SetStripPrefix configures a path prefix to strip from endpoint paths
// (e.g. "/api") so callers work with shorter paths.
func (c *Cache) SetStripPrefix(prefix string) {
	c.stripPrefix = prefix
}

// NewCache creates a new OpenAPI cache with default settings.
func NewCache() *Cache {
	return &Cache{
		specURL:   SpecURL,
		cachePath: filepath.Join(config.HomeConfigPath, CloudCacheFileName),
	}
}

// NewCacheWithOptions creates a new OpenAPI cache with custom settings.
func NewCacheWithOptions(specURL, cachePath string) *Cache {
	return &Cache{
		specURL:   specURL,
		cachePath: cachePath,
	}
}

// NewCacheWithAuth creates a new OpenAPI cache that sends an auth token when fetching specs.
func NewCacheWithAuth(specURL, cachePath, authToken string) *Cache {
	return &Cache{
		specURL:   specURL,
		cachePath: cachePath,
		authToken: authToken,
	}
}

// SpecCacheFileName returns a deterministic cache file name derived from a spec URL.
func SpecCacheFileName(specURL string) string {
	h := sha256.Sum256([]byte(specURL))
	return fmt.Sprintf("openapi-cache-%x.json", h[:8])
}

// NewAirflowCacheForVersion creates a new OpenAPI cache configured for a specific Airflow version.
// It automatically determines the correct spec URL and cache file path based on the version.
// Endpoint paths are stripped of their API version prefix (/api/v1 or /api/v2) so that
// callers work with version-agnostic paths like /dags instead of /api/v2/dags.
func NewAirflowCacheForVersion(version string) (*Cache, error) {
	specURL, err := BuildAirflowSpecURL(version)
	if err != nil {
		return nil, fmt.Errorf("building spec URL for version %s: %w", version, err)
	}

	cachePath := filepath.Join(config.HomeConfigPath, AirflowCacheFileNameForVersion(version))

	// Determine the API prefix to strip based on major version
	prefix := "/api/v2"
	normalized := NormalizeAirflowVersion(version)
	if strings.HasPrefix(normalized, "2.") {
		prefix = "/api/v1"
	}

	return &Cache{
		specURL:     specURL,
		cachePath:   cachePath,
		stripPrefix: prefix,
	}, nil
}

// Load loads the OpenAPI spec, using cache if valid or fetching if needed.
// If forceRefresh is true, the cache is ignored and a fresh spec is fetched.
func (c *Cache) Load(forceRefresh bool) error {
	if !forceRefresh {
		// Try to load from cache first
		if err := c.readCache(); err == nil && !c.isExpired() {
			return nil
		}
	}

	// Fetch fresh spec
	if err := c.fetchSpec(); err != nil {
		// If fetch fails and we have a stale cache, use it
		if c.IsLoaded() {
			return nil
		}
		return err
	}

	// Save to cache
	return c.saveCache()
}

// GetSpecURL returns the URL used to fetch the OpenAPI spec.
func (c *Cache) GetSpecURL() string {
	return c.specURL
}

// GetDoc returns the loaded OpenAPI document.
func (c *Cache) GetDoc() *v3high.Document {
	return c.doc
}

// GetEndpoints extracts all endpoints from the loaded spec.
// If a stripPrefix is configured, it is removed from each endpoint path.
func (c *Cache) GetEndpoints() []Endpoint {
	var endpoints []Endpoint
	switch {
	case c.doc != nil:
		endpoints = ExtractEndpoints(c.doc)
	case c.v2doc != nil:
		endpoints = ExtractEndpointsV2(c.v2doc)
	default:
		return nil
	}
	if c.stripPrefix != "" {
		for i := range endpoints {
			endpoints[i].Path = strings.TrimPrefix(endpoints[i].Path, c.stripPrefix)
		}
	}
	return endpoints
}

// IsLoaded returns true if a spec has been loaded.
func (c *Cache) IsLoaded() bool {
	return c.doc != nil || c.v2doc != nil
}

// GetServerPath returns the base path from the loaded spec.
// For OpenAPI 3.x it extracts the path from servers[0].url.
// For Swagger 2.0 it returns the basePath field.
func (c *Cache) GetServerPath() string {
	if c.doc != nil && len(c.doc.Servers) > 0 {
		u, err := url.Parse(c.doc.Servers[0].URL)
		if err == nil {
			return strings.TrimSuffix(u.Path, "/")
		}
	}
	if c.v2doc != nil {
		return strings.TrimSuffix(c.v2doc.BasePath, "/")
	}
	return ""
}

// readCache attempts to read the cached spec from disk.
func (c *Cache) readCache() error {
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return err
	}

	var cached CachedSpec
	if err := json.Unmarshal(data, &cached); err != nil {
		return err
	}

	v3doc, v2doc, err := parseSpec(cached.RawSpec)
	if err != nil {
		return err
	}

	c.doc = v3doc
	c.v2doc = v2doc
	c.rawSpec = cached.RawSpec
	c.fetchedAt = cached.FetchedAt
	return nil
}

// saveCache saves the current spec to disk.
func (c *Cache) saveCache() error {
	cached := CachedSpec{
		RawSpec:   c.rawSpec,
		FetchedAt: c.fetchedAt,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(c.cachePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return err
	}

	return os.WriteFile(c.cachePath, data, filePermissions)
}

// isExpired returns true if the cached spec has expired.
func (c *Cache) isExpired() bool {
	return time.Since(c.fetchedAt) > CacheTTL
}

// fetchSpec fetches the OpenAPI spec from the remote URL.
func (c *Cache) fetchSpec() error {
	ctx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.specURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Accept both JSON and YAML
	req.Header.Set("Accept", "application/json, application/yaml, text/yaml, */*")

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("fetching OpenAPI spec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	v3doc, v2doc, err := parseSpec(body)
	if err != nil {
		return fmt.Errorf("parsing OpenAPI spec: %w", err)
	}

	c.doc = v3doc
	c.v2doc = v2doc
	c.rawSpec = body
	c.fetchedAt = time.Now()
	return nil
}

// parseSpec parses raw bytes into an OpenAPI document using libopenapi.
// It detects the spec version and returns either a v3 or v2 model.
// This handles both JSON and YAML formats and resolves $ref references.
func parseSpec(data []byte) (*v3high.Document, *v2high.Swagger, error) {
	doc, err := newDocument(data)
	if err != nil {
		return nil, nil, fmt.Errorf("creating document: %w", err)
	}

	version := doc.GetVersion()
	if strings.HasPrefix(version, "2.") {
		model, err := doc.BuildV2Model()
		if err != nil {
			return nil, nil, fmt.Errorf("building v2 model: %w", err)
		}
		if model == nil {
			return nil, nil, fmt.Errorf("building v2 model: unknown error")
		}
		return nil, &model.Model, nil
	}

	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, nil, fmt.Errorf("building v3 model: %w", err)
	}
	if model == nil {
		return nil, nil, fmt.Errorf("building v3 model: unknown error")
	}
	return &model.Model, nil, nil
}

// ClearCache removes the cached spec file.
func (c *Cache) ClearCache() error {
	return os.Remove(c.cachePath)
}
