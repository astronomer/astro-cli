package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAirflowRequirement(t *testing.T) {
	cases := map[string]string{
		"3":     "apache-airflow==3.*",
		"3.1":   "apache-airflow==3.1.*",
		"2.9":   "apache-airflow==2.9.*",
		"3.1.2": "apache-airflow==3.1.2",
	}
	for version, want := range cases {
		assert.Equal(t, want, airflowRequirement(version), version)
	}
}

func TestPinFromSpec(t *testing.T) {
	pinned := map[string]string{
		"apache-airflow==2.9.3":                        "2.9.3",
		"apache-airflow==2.9":                          "2.9",
		"apache-airflow==3":                            "3",
		"apache-airflow[celery]==2.9.3":                "2.9.3",
		"apache_airflow==2.9.3":                        "2.9.3",
		"APACHE-AIRFLOW==2.9.3":                        "2.9.3",
		"apache-airflow ==2.9.3":                       "2.9.3",
		`apache-airflow==2.9.3; python_version<"3.12"`: "2.9.3",
		"apache-airflow[celery,statsd]==2.9.3 ; sys_platform == 'linux'": "2.9.3",
	}
	for spec, want := range pinned {
		got, ok := pinFromSpec(spec)
		assert.True(t, ok, spec)
		assert.Equal(t, want, got, spec)
	}

	// A version this cannot read exactly yields no pin, and the caller falls
	// back to the default rather than guessing which release is meant.
	unpinned := []string{
		"apache-airflow>=2.9,<3",
		"apache-airflow==2.9.*",
		"apache-airflow~=2.9.3",
		"apache-airflow",
		"apache-airflow==2.9.3,!=2.9.4",
		"apache-airflow @ https://example.com/airflow.whl",
		"apache-airflow-providers-snowflake==5.1.0",
		"pandas==2.0.0",
		"",
	}
	for _, spec := range unpinned {
		_, ok := pinFromSpec(spec)
		assert.False(t, ok, spec)
	}
}

func TestNamesAirflow(t *testing.T) {
	for _, spec := range []string{
		"apache-airflow",
		"apache-airflow==2.9.3",
		"apache_airflow>=2.9",
		"apache-airflow[celery]==2.9.3",
		"APACHE-AIRFLOW==2.9.3",
	} {
		assert.True(t, namesAirflow(spec), spec)
	}
	// The providers are their own distributions, and a name that merely
	// contains "airflow" is not the core one.
	for _, spec := range []string{
		"apache-airflow-providers-snowflake==5.1.0",
		"apache-airflow-task-sdk",
		"airflow-exporter",
		"pandas",
	} {
		assert.False(t, namesAirflow(spec), spec)
	}
}

func TestResolveAirflowVersion(t *testing.T) {
	// The flag wins over anything the manifest says.
	version, defaulted := resolveAirflowVersion("2.10", []string{"apache-airflow==2.9.3"})
	assert.Equal(t, "2.10", version)
	assert.False(t, defaulted)

	// Then a clean pin already in the manifest, so a project on 2.9 stays there.
	version, defaulted = resolveAirflowVersion("", []string{"pandas", "apache-airflow==2.9.3"})
	assert.Equal(t, "2.9.3", version)
	assert.False(t, defaulted)

	// Then the default, reported as such.
	version, defaulted = resolveAirflowVersion("", []string{"apache-airflow>=2.9,<3"})
	assert.Equal(t, DefaultAirflowVersion, version)
	assert.True(t, defaulted)

	version, defaulted = resolveAirflowVersion("", nil)
	assert.Equal(t, DefaultAirflowVersion, version)
	assert.True(t, defaulted)
}

func TestPinsAirflow(t *testing.T) {
	assert.True(t, pinsAirflow([]string{"pandas", "apache-airflow[celery]>=2.9"}))
	assert.False(t, pinsAirflow([]string{"pandas", "apache-airflow-providers-snowflake==5.1.0"}))
	assert.False(t, pinsAirflow(nil))
}
