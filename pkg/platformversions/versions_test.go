package platformversions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve(t *testing.T) {
	// Synthetic versions, so the test exercises the resolver without pinning to
	// the real platform data (which the data-guard test covers).
	supported := []Version{
		{Airflow: "7.2.1", Python: py312},
		{Airflow: "7.0.6", Python: py312},
		{Airflow: "6.11.0", Python: py311},
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
		v, ok := Resolve(tc.pin, supported)
		assert.Equal(t, tc.wantOK, ok, tc.describe)
		assert.Equal(t, tc.wantVer, v.Airflow, tc.describe)
		assert.Equal(t, tc.wantPy, v.Python, tc.describe)
	}
}

func TestResolvePartialDoesNotMatchAcrossMinor(t *testing.T) {
	// "8.1" must not match "8.10.x" or "8.12.x": the prefix carries the dot.
	supported := []Version{{Airflow: "8.12.0"}, {Airflow: "8.10.1"}}
	_, ok := Resolve("8.1", supported)
	assert.False(t, ok, `"8.1" should not match "8.10" or "8.12"`)
}

func TestMatch(t *testing.T) {
	supported := []Version{
		{Airflow: "7.2.1", Python: py312},
		{Airflow: "7.0.6", Python: py312},
		{Airflow: "6.11.0", Python: py311},
	}
	cases := []struct {
		pin           string
		wantVer       string
		wantDowngrade bool
		wantOK        bool
		describe      string
	}{
		{"7.0.6", "7.0.6", false, true, "exact pin, no downgrade"},
		{"7", "7.2.1", false, true, "partial pin resolves within band"},
		{"7.1", "7.0.6", true, true, "unsupported minor maps down to newest below it"},
		{"8.0", "7.2.1", true, true, "pin above the top maps to the newest offered"},
		{"7.5", "7.2.1", true, true, "unsupported patch band maps down"},
		{"6.11.0", "6.11.0", false, true, "exact lowest, no downgrade"},
		{"5", "", false, false, "pin below the floor has nothing to map to"},
		{"6.0", "", false, false, "below the lowest offered is an error"},
	}
	for _, tc := range cases {
		v, downgraded, ok := Match(tc.pin, supported)
		assert.Equal(t, tc.wantOK, ok, tc.describe)
		assert.Equal(t, tc.wantDowngrade, downgraded, tc.describe)
		assert.Equal(t, tc.wantVer, v.Airflow, tc.describe)
	}
}

// TestMatchRealData pins the two smoke-test cases from the live report: 3.1 on
// Composer resolves cleanly to 3.1.8, and 3.1 on MWAA maps down to 3.0.6.
func TestMatchRealData(t *testing.T) {
	v, down, ok := Match("3.1", Composer)
	assert.True(t, ok)
	assert.False(t, down, "Composer offers 3.1.x")
	assert.Equal(t, "3.1.8", v.Airflow)

	v, down, ok = Match("3.1", MWAA)
	assert.True(t, ok)
	assert.True(t, down, "MWAA does not offer 3.1, so it maps down")
	assert.Equal(t, "3.0.6", v.Airflow)

	// 3.3 is beyond both platforms today; it maps down to each one's newest.
	v, down, _ = Match("3.3", MWAA)
	assert.True(t, down)
	assert.Equal(t, "3.2.1", v.Airflow)
	v, down, _ = Match("3.3", Composer)
	assert.True(t, down)
	assert.Equal(t, "3.1.8", v.Airflow)
}

func TestList(t *testing.T) {
	got := List([]Version{{Airflow: "7.2.1"}, {Airflow: "7.0.6"}})
	assert.Equal(t, "7.2.1, 7.0.6", got)
}

func TestMWAAConstraintURL(t *testing.T) {
	got := MWAAConstraintURL(Version{Airflow: "3.0.6", Python: "3.12"})
	assert.Equal(t, "https://raw.githubusercontent.com/apache/airflow/constraints-3.0.6/constraints-3.12.txt", got)
}

// TestPlatformDataResolves guards the shipped data: every platform's own
// newest version must resolve against its list, so a bad edit to the tables is
// caught.
func TestPlatformDataResolves(t *testing.T) {
	for _, supported := range [][]Version{MWAA, Composer} {
		newest := supported[0]
		v, ok := Resolve(newest.Airflow, supported)
		assert.True(t, ok)
		assert.Equal(t, newest.Airflow, v.Airflow)
	}
}
