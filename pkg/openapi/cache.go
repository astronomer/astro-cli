package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
	"gopkg.in/yaml.v3"
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

// CachedSpec wraps the OpenAPI spec with metadata for caching.
type CachedSpec struct {
	Spec      *OpenAPISpec `json:"spec"`
	FetchedAt time.Time    `json:"fetchedAt"`
}

// Cache manages fetching and caching of an OpenAPI specification.
type Cache struct {
	specURL     string
	cachePath   string
	stripPrefix string // optional prefix to strip from endpoint paths (e.g. "/api/v2")
	httpClient  *http.Client
	spec        *OpenAPISpec
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
		if c.spec != nil {
			return nil
		}
		return err
	}

	// Save to cache
	return c.saveCache()
}

// GetSpec returns the loaded OpenAPI spec.
func (c *Cache) GetSpec() *OpenAPISpec {
	return c.spec
}

// GetEndpoints extracts all endpoints from the loaded spec.
// If a stripPrefix is configured, it is removed from each endpoint path.
func (c *Cache) GetEndpoints() []Endpoint {
	if c.spec == nil {
		return nil
	}
	endpoints := ExtractEndpoints(c.spec)
	if c.stripPrefix != "" {
		for i := range endpoints {
			endpoints[i].Path = strings.TrimPrefix(endpoints[i].Path, c.stripPrefix)
		}
	}
	return endpoints
}

// IsLoaded returns true if a spec has been loaded.
func (c *Cache) IsLoaded() bool {
	return c.spec != nil
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

	c.spec = cached.Spec
	c.fetchedAt = cached.FetchedAt
	return nil
}

// saveCache saves the current spec to disk.
func (c *Cache) saveCache() error {
	cached := CachedSpec{
		Spec:      c.spec,
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

	var spec OpenAPISpec
	contentType := resp.Header.Get("Content-Type")

	// Try to parse based on content type, fallback to trying both formats
	switch {
	case strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml"):
		if err := yaml.Unmarshal(body, &spec); err != nil {
			return fmt.Errorf("parsing OpenAPI spec as YAML: %w", err)
		}
	case strings.Contains(contentType, "json"):
		if err := json.Unmarshal(body, &spec); err != nil {
			return fmt.Errorf("parsing OpenAPI spec as JSON: %w", err)
		}
	default:
		// Content-Type not helpful, try YAML first (more common for OpenAPI), then JSON
		if err := yaml.Unmarshal(body, &spec); err != nil {
			if err := json.Unmarshal(body, &spec); err != nil {
				return fmt.Errorf("parsing OpenAPI spec (tried YAML and JSON): %w", err)
			}
		}
	}

	c.spec = &spec
	c.fetchedAt = time.Now()
	return nil
}

// ClearCache removes the cached spec file.
func (c *Cache) ClearCache() error {
	return os.Remove(c.cachePath)
}
