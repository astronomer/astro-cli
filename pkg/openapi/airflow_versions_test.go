package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSpecPathForVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantPath    string
		wantErr     bool
		errContains string
	}{
		// Airflow 2.x versions
		{
			name:     "Airflow 2.0.0",
			version:  "2.0.0",
			wantPath: "airflow/api_connexion/openapi/v1.yaml", //nolint:misspell // real Airflow module name
		},
		{
			name:     "Airflow 2.5.0",
			version:  "2.5.0",
			wantPath: "airflow/api_connexion/openapi/v1.yaml", //nolint:misspell // real Airflow module name
		},
		{
			name:     "Airflow 2.10.0",
			version:  "2.10.0",
			wantPath: "airflow/api_connexion/openapi/v1.yaml", //nolint:misspell // real Airflow module name
		},
		{
			name:     "Airflow 2.11.0",
			version:  "2.11.0",
			wantPath: "airflow/api_connexion/openapi/v1.yaml", //nolint:misspell // real Airflow module name
		},
		// Airflow 3.0.x early versions (v1 spec)
		{
			name:     "Airflow 3.0.0",
			version:  "3.0.0",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v1-rest-api-generated.yaml",
		},
		{
			name:     "Airflow 3.0.1",
			version:  "3.0.1",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v1-rest-api-generated.yaml",
		},
		{
			name:     "Airflow 3.0.2",
			version:  "3.0.2",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v1-rest-api-generated.yaml",
		},
		// Airflow 3.0.3+ (v2 spec)
		{
			name:     "Airflow 3.0.3",
			version:  "3.0.3",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:     "Airflow 3.0.6",
			version:  "3.0.6",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:     "Airflow 3.1.0",
			version:  "3.1.0",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:     "Airflow 3.1.7",
			version:  "3.1.7",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		// Future versions (should use latest range)
		{
			name:     "Future version 3.5.0",
			version:  "3.5.0",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:     "Future version 4.0.0 (fallback to latest)",
			version:  "4.0.0",
			wantPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		// Invalid versions
		{
			name:        "Invalid version",
			version:     "not-a-version",
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name:        "Empty version",
			version:     "",
			wantErr:     true,
			errContains: "invalid version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSpecPathForVersion(tt.version)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestBuildAirflowSpecURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantURL string
		wantErr bool
	}{
		{
			name:    "Airflow 2.10.0",
			version: "2.10.0",
			wantURL: "https://raw.githubusercontent.com/apache/airflow/refs/tags/2.10.0/airflow/api_connexion/openapi/v1.yaml",
		},
		{
			name:    "Airflow 3.0.0",
			version: "3.0.0",
			wantURL: "https://raw.githubusercontent.com/apache/airflow/refs/tags/3.0.0/airflow-core/src/airflow/api_fastapi/core_api/openapi/v1-rest-api-generated.yaml",
		},
		{
			name:    "Airflow 3.1.7",
			version: "3.1.7",
			wantURL: "https://raw.githubusercontent.com/apache/airflow/refs/tags/3.1.7/airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:    "Airflow 3.1.7 with build metadata",
			version: "3.1.7+astro.1",
			wantURL: "https://raw.githubusercontent.com/apache/airflow/refs/tags/3.1.7/airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml",
		},
		{
			name:    "Invalid version",
			version: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildAirflowSpecURL(tt.version)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, got)
		})
	}
}

func TestAirflowCacheFileNameForVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2.10.0", "openapi-airflow-2.10.0-cache.json"},
		{"3.0.0", "openapi-airflow-3.0.0-cache.json"},
		{"3.1.7", "openapi-airflow-3.1.7-cache.json"},
		{"3.1.7+astro.1", "openapi-airflow-3.1.7-cache.json"}, // Build metadata stripped
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := AirflowCacheFileNameForVersion(tt.version)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNormalizeAirflowVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"3.1.7", "3.1.7"},
		{"3.1.7+astro.1", "3.1.7"},
		{"2.10.0+custom.build", "2.10.0"},
		{"3.0.0-beta+build123", "3.0.0-beta"},
		{"2.5.0", "2.5.0"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := NormalizeAirflowVersion(tt.version)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestVersionRangesAreSorted(t *testing.T) {
	// Verify that version ranges are properly ordered (oldest to newest)
	require.Greater(t, len(AirflowVersionRanges), 0, "Should have at least one version range")

	// Verify ranges are sorted by checking that each range's Version is > the previous
	for i := 1; i < len(AirflowVersionRanges); i++ {
		prev := AirflowVersionRanges[i-1]
		curr := AirflowVersionRanges[i]

		assert.Less(t, prev.Version, curr.Version,
			"Version ranges should be sorted: %s should be < %s",
			prev.Version, curr.Version)
	}
}
