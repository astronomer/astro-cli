package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSweepable(t *testing.T) {
	for name, want := range map[string]bool{
		"ASTRO_UV_BIN":                 true,
		"ASTRO_AIRFLOW_INDEX_URL":      true,
		"astro_api_token":              true,
		"ASTRONOMER_KEY_ID":            true,
		"ASTRONOMER_ANYTHING":          true,
		"ASTRO_TEST_STRICT_REALCONFIG": false,
		"ASTRO_TEST_ISOLATED_HOME":     false,
		"ASTRO_TEST_ANYTHING":          false,
		"ASTRO_UPDATE_SCHEMAS":         false,
		"astro_update_schemas":         false,
		"PATH":                         false,
		"HOME":                         false,
		"CI":                           false, // the list clears it, not the sweep
	} {
		if got := sweepable(name); got != want {
			t.Errorf("sweepable(%q) = %v, want %v", name, got, want)
		}
	}
}

// What the sweep does to the environment of a run: the CLI's variables go, the
// test controls and the helper marker stay. ASTRO_UPDATE_SCHEMAS surviving is
// what keeps `make update-schemas` rewriting goldens.
func TestTheSweepClearsTheCLIsVariablesAndKeepsTheTestControls(t *testing.T) {
	for k, v := range map[string]string{
		"ASTRO_TEST_SOMETHING": "1",
		"ASTRO_UPDATE_SCHEMAS": "1",
		"ASTRO_UV_BIN":         "/custom/uv",
		"ASTRONOMER_KEY_ID":    "id",
		"ASTRO_API_TOKEN":      "token",
	} {
		t.Setenv(k, v)
	}
	driveIsolate(t, guards{watch: newStateWatch(t.TempDir()), refusals: none})

	for _, k := range []string{"ASTRO_TEST_SOMETHING", "ASTRO_UPDATE_SCHEMAS"} {
		if os.Getenv(k) != "1" {
			t.Errorf("%s did not survive the sweep", k)
		}
	}
	for _, k := range []string{"ASTRO_UV_BIN", "ASTRONOMER_KEY_ID", "ASTRO_API_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			t.Errorf("%s = %q survived the sweep", k, v)
		}
	}
	if os.Getenv(inheritedEnv) == "" {
		t.Error("the helper marker did not survive the sweep")
	}
}

// A helper's marker is a home a test binary made, in the temporary directory:
// not the temporary directory itself, nor anything outside it.
func TestStrictlyWithin(t *testing.T) {
	tmp := t.TempDir()
	for path, want := range map[string]bool{
		filepath.Join(tmp, "home"):   true,
		filepath.Join(tmp, "a", "b"): true,
		tmp:                          false,
		filepath.Dir(tmp):            false,
		"":                           false,
	} {
		if got := strictlyWithin(path, tmp); got != want {
			t.Errorf("strictlyWithin(%q, tmp) = %v, want %v", path, got, want)
		}
	}
}
