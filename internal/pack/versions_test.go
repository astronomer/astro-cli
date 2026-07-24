package pack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVersion(t *testing.T) {
	// Synthetic versions, so the test exercises the resolver without pinning to
	// the real platform data (which the target tests cover).
	supported := []platformVersion{
		{airflow: "7.2.1", python: py312},
		{airflow: "7.0.6", python: py312},
		{airflow: "6.11.0", python: py311},
	}
	cases := []struct {
		pin      string
		wantVer  string
		wantOK   bool
		wantPy   string
		describe string
	}{
		{"7.0.6", "7.0.6", true, py312, "exact match"},
		{"7", "7.2.1", true, py312, "partial pin takes the newest 7.x"},
		{"6", "6.11.0", true, py311, "partial pin takes the newest 6.x"},
		{"7.1", "", false, "", "no 7.1 offered"},
		{"9.0.0", "", false, "", "unknown major"},
		{" 7.0.6 ", "7.0.6", true, py312, "trims whitespace"},
	}
	for _, tc := range cases {
		v, ok := resolveVersion(tc.pin, supported)
		assert.Equal(t, tc.wantOK, ok, tc.describe)
		assert.Equal(t, tc.wantVer, v.airflow, tc.describe)
		assert.Equal(t, tc.wantPy, v.python, tc.describe)
	}
}

func TestResolveVersionPartialDoesNotMatchAcrossMinor(t *testing.T) {
	// "8.1" must not match "8.10.x" or "8.12.x": the prefix carries the dot.
	supported := []platformVersion{{airflow: "8.12.0"}, {airflow: "8.10.1"}}
	_, ok := resolveVersion("8.1", supported)
	assert.False(t, ok, `"8.1" should not match "8.10" or "8.12"`)
}

func TestSupportedList(t *testing.T) {
	got := supportedList([]platformVersion{{airflow: "7.2.1"}, {airflow: "7.0.6"}})
	assert.Equal(t, "7.2.1, 7.0.6", got)
}

// TestPlatformDataResolves guards the shipped data: every platform's own newest
// version must resolve against its list, so a bad edit to the tables is caught.
func TestPlatformDataResolves(t *testing.T) {
	for _, supported := range [][]platformVersion{mwaaAirflowVersions, composerAirflowVersions} {
		newest := supported[0]
		v, ok := resolveVersion(newest.airflow, supported)
		assert.True(t, ok)
		assert.Equal(t, newest.airflow, v.airflow)
	}
}
