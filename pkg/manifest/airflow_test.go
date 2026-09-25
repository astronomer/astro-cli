package manifest

import (
	"errors"
	"reflect"
	"testing"
)

func TestAirflowRequirement(t *testing.T) {
	cases := map[string]string{
		"3":     "apache-airflow==3.*",
		"3.1":   "apache-airflow==3.1.*",
		"2.9":   "apache-airflow==2.9.*",
		"3.1.2": "apache-airflow==3.1.2",
	}
	for version, want := range cases {
		if got := AirflowRequirement(version); got != want {
			t.Errorf("AirflowRequirement(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestAirflowPin(t *testing.T) {
	pinned := map[string]string{
		"apache-airflow==2.9.3": "2.9.3",
		// Without a wildcard, "==" is one release, padded with zeros the way
		// PEP 440 compares it: ==2.9 is 2.9.0, not the newest 2.9.
		"apache-airflow==2.9":                                            "2.9.0",
		"apache-airflow==3":                                              "3.0.0",
		"apache-airflow[celery]==2.9.3":                                  "2.9.3",
		"apache_airflow==2.9.3":                                          "2.9.3",
		"APACHE-AIRFLOW==2.9.3":                                          "2.9.3",
		"apache-airflow ==2.9.3":                                         "2.9.3",
		`apache-airflow==2.9.3; python_version<"3.12"`:                   "2.9.3",
		"apache-airflow[celery,statsd]==2.9.3 ; sys_platform == 'linux'": "2.9.3",
		// A series, the shape AirflowRequirement writes.
		"apache-airflow==2.9.*":                           "2.9",
		"apache-airflow==3.*":                             "3",
		"apache-airflow[celery]==3.1.*":                   "3.1",
		"apache-airflow==3.1.* ; sys_platform == 'linux'": "3.1",
		// The core distribution states the version the same way.
		"apache-airflow-core==3.3.*":                        "3.3",
		"apache-airflow-core==3.3.2":                        "3.3.2",
		"apache_airflow_core[otel]==3.2.*":                  "3.2",
		"Apache-Airflow-Core == 3.3.* ; os_name == 'posix'": "3.3",
	}
	for spec, want := range pinned {
		got, ok := AirflowPin(spec)
		if !ok || got != want {
			t.Errorf("AirflowPin(%q) = %q, %v; want %q, true", spec, got, ok, want)
		}
	}

	// A version that names no single series yields no pin.
	unpinned := []string{
		"apache-airflow>=2.9,<3",
		"apache-airflow~=2.9.3",
		"apache-airflow==2.*.3",
		// Another specifier BEFORE the "==". strings.Cut keeps only what
		// follows the first one, so these reach the version check as a clean
		// "2.9.*" with the rest discarded.
		"apache-airflow>=2.9,==2.9.*",
		"apache-airflow<3,==2.10.*",
		"apache-airflow!=2.9.1,==2.9.*",
		"apache-airflow[celery]>=2.9,==2.9.*",
		"apache-airflow",
		"apache-airflow==2.9.3,!=2.9.4",
		"apache-airflow @ https://example.com/airflow.whl",
		"apache-airflow-core>=3.1",
		"apache-airflow-core @ file:///wheels/apache_airflow_core-3.3.2-py3-none-any.whl",
		"apache-airflow-providers-snowflake==5.1.0",
		"apache-airflow-task-sdk==1.1.0",
		"pandas==2.0.0",
		"",
	}
	for _, spec := range unpinned {
		if got, ok := AirflowPin(spec); ok {
			t.Errorf("AirflowPin(%q) = %q, true; want no pin", spec, got)
		}
	}
}

// Every requirement AirflowRequirement writes, AirflowPin reads back as the
// version it was given.
func TestAirflowPinReadsWhatAirflowRequirementWrites(t *testing.T) {
	for _, version := range []string{"2", "2.9", "2.10", "3", "3.1", "3.1.2", "3.1.0"} {
		spec := AirflowRequirement(version)
		if got, ok := AirflowPin(spec); !ok || got != version {
			t.Errorf("%s -> %s read back as %q, %v", version, spec, got, ok)
		}
	}
}

func TestNamesAirflow(t *testing.T) {
	for _, spec := range []string{
		"apache-airflow",
		"apache-airflow==2.9.3",
		"apache_airflow>=2.9",
		"apache-airflow[celery]==2.9.3",
		"APACHE-AIRFLOW==2.9.3",
		"apache-airflow-core==3.3.*",
		"apache_airflow_core>=3.1",
	} {
		if !NamesAirflow(spec) {
			t.Errorf("NamesAirflow(%q) = false, want true", spec)
		}
	}
	// The providers and the task SDK are their own distributions, and a name
	// that merely contains "airflow" is not one of the two.
	for _, spec := range []string{
		"apache-airflow-providers-snowflake==5.1.0",
		"apache-airflow-task-sdk",
		"apache-airflow-core-extras",
		"airflow-exporter",
		"pandas",
	} {
		if NamesAirflow(spec) {
			t.Errorf("NamesAirflow(%q) = true, want false", spec)
		}
	}
}

func TestAirflowSeriesAndMajor(t *testing.T) {
	cases := []struct{ pin, series, major string }{
		{"3.3", "3.3", "3"},
		{"3.3.2", "3.3", "3"},
		{"3", "3", "3"},
		{"2.10.5", "2.10", "2"},
		{"", "", ""},
	}
	for _, tc := range cases {
		a := Airflow{Pin: tc.pin}
		if got := a.Series(); got != tc.series {
			t.Errorf("Airflow{%q}.Series() = %q, want %q", tc.pin, got, tc.series)
		}
		if got := a.Major(); got != tc.major {
			t.Errorf("Airflow{%q}.Major() = %q, want %q", tc.pin, got, tc.major)
		}
	}
}

// ReadAirflowStatement answers the Airflow question whatever else is wrong,
// and fails only when there is nothing to read.
func TestReadAirflowStatement(t *testing.T) {
	cases := map[string]struct {
		data string
		want AirflowStatement
	}{
		"clean":                      {string(airflowManifest(`'apache-airflow==3.1.*'`, "")), AirflowStatement{Requirement: "3.1"}},
		"leftover key that agrees":   {string(airflowManifest(`'apache-airflow==3.1.*'`, "airflow = '3.1'\n")), AirflowStatement{Requirement: "3.1", Key: "3.1"}},
		"leftover key that does not": {string(airflowManifest(`'apache-airflow==3.1.*'`, "airflow = '2.10'\n")), AirflowStatement{Requirement: "3.1", Key: "2.10"}},
		"key and a range":            {string(airflowManifest(`'apache-airflow>=3'`, "airflow = '2.10'\n")), AirflowStatement{Key: "2.10"}},
		"an unrelated problem":       {string(airflowManifest(`'apache-airflow==2.10.*'`, "airflw = 'x'\n")), AirflowStatement{Requirement: "2.10"}},
		"two pins that disagree":     {string(airflowManifest(`"apache-airflow==3.1.*; python_version < '3.12'", "apache-airflow==3.2.*; python_version >= '3.12'"`, "")), AirflowStatement{}},
		"no project table":           {"[tool.astro]\nairflow = '2.9'\n", AirflowStatement{Key: "2.9"}},
	}
	for name, tc := range cases {
		got, err := ReadAirflowStatement([]byte(tc.data))
		if err != nil || got != tc.want {
			t.Errorf("%s: ReadAirflowStatement = %#v, %v; want %#v", name, got, err, tc.want)
		}
	}
	if _, err := ReadAirflowStatement([]byte("[project]\nname = 'p'\n")); !errors.Is(err, ErrNoAstroSection) {
		t.Errorf("no [tool.astro]: err = %v, want ErrNoAstroSection", err)
	}
	var pe *ParseError
	if _, err := ReadAirflowStatement([]byte("[project\n")); !errors.As(err, &pe) {
		t.Errorf("not TOML: err = %v, want a *ParseError", err)
	}
}

func TestWithoutAirflow(t *testing.T) {
	got := WithoutAirflow([]string{
		"apache-airflow==3.1.*",
		"pandas",
		"Apache_Airflow_Core[otel]==3.3.2 ; sys_platform == 'linux'",
		"apache-airflow-providers-standard",
		"apache-airflow-task-sdk>=1.1",
	})
	want := []string{"pandas", "apache-airflow-providers-standard", "apache-airflow-task-sdk>=1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WithoutAirflow = %q, want %q", got, want)
	}
}

// Manifest.Airflow reads the requirement, not a key of its own, and skips the
// distributions that only share the prefix.
func TestManifestAirflowReadsTheRequirement(t *testing.T) {
	cases := []struct {
		name string
		deps []string
		want string
	}{
		{"series", []string{"pandas", "apache-airflow==3.1.*"}, "3.1"},
		{"patch", []string{"apache-airflow==3.1.2"}, "3.1.2"},
		{"core", []string{"apache-airflow-core==3.3.*"}, "3.3"},
		{"extras and marker", []string{"apache-airflow[celery]==2.10.* ; sys_platform == 'linux'"}, "2.10"},
		{"providers are not airflow", []string{"apache-airflow-providers-snowflake==5.1.0", "apache-airflow==3.2.*"}, "3.2"},
		{"none", []string{"pandas"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{Project: Project{Dependencies: tc.deps}}
			if got := m.Airflow().Pin; got != tc.want {
				t.Errorf("Airflow().Pin = %q, want %q", got, tc.want)
			}
		})
	}
}

// airflowManifest is a minimal manifest with this dependency list and this
// [tool.astro] body.
func airflowManifest(deps, astro string) []byte {
	return []byte("[project]\nname = 'p'\ndependencies = [" + deps + "]\n\n[tool.astro]\n" + astro)
}

// A loaded manifest's version is the requirement's, in each shape it takes.
func TestParsedManifestAirflow(t *testing.T) {
	cases := map[string]string{
		`'apache-airflow==3.3.*'`:                    "3.3",
		`'apache-airflow==3.3.2'`:                    "3.3.2",
		`'apache-airflow==3.3'`:                      "3.3.0",
		`'apache-airflow-core==3.2.*'`:               "3.2",
		`'apache-airflow[celery]==2.10.*', 'pandas'`: "2.10",
		// The same pin under two markers is one pin.
		`'apache-airflow==3.1.*; python_version >= "3.11"', 'apache-airflow==3.1.*; python_version < "3.11"'`: "3.1",
	}
	for deps, want := range cases {
		m, err := Parse(airflowManifest(deps, ""))
		if err != nil {
			t.Errorf("%s: %v", deps, err)
			continue
		}
		if got := m.Airflow().Pin; got != want {
			t.Errorf("%s: Airflow().Pin = %q, want %q", deps, got, want)
		}
	}
}

// The reasons are asserted whole here, once, because they are the fix a user
// reads: each one names the line and what to write instead.
func TestAirflowProblemReasons(t *testing.T) {
	cases := []struct {
		name        string
		deps, astro string
		want        []Problem
	}{
		{
			name: "range",
			deps: `'apache-airflow>=3.1'`,
			want: []Problem{{
				CodeAirflowUnpinned, "project.dependencies[0]",
				"apache-airflow>=3.1 does not pin an Airflow series. Pin one, like apache-airflow==3.3.*",
			}},
		},
		{
			name: "core range",
			deps: `'pandas', 'apache-airflow-core~=3.2'`,
			want: []Problem{{
				CodeAirflowUnpinned, "project.dependencies[1]",
				"apache-airflow-core~=3.2 does not pin an Airflow series. Pin one, like apache-airflow-core==3.3.*",
			}},
		},
		{
			name: "none",
			deps: `'pandas'`,
			want: []Problem{{
				CodeAirflowMissing, "project.dependencies",
				"names no Airflow. Add one, like apache-airflow==3.3.*",
			}},
		},
		{
			name:  "leftover key, same series",
			deps:  `'apache-airflow==3.1.*'`,
			astro: "airflow = '3.1'\n",
			want: []Problem{{
				CodeAirflowRemoved, "tool.astro.airflow",
				"is no longer read: the Airflow version is the requirement apache-airflow==3.1.* in [project] dependencies. Delete the airflow line.",
			}},
		},
		{
			// A patch under the same series is the same series: the key
			// said nothing the requirement does not.
			name:  "leftover key, a patch of the same series",
			deps:  `'apache-airflow==3.1.*'`,
			astro: "airflow = '3.1.2'\n",
			want: []Problem{{
				CodeAirflowRemoved, "tool.astro.airflow",
				"is no longer read: the Airflow version is the requirement apache-airflow==3.1.* in [project] dependencies. Delete the airflow line.",
			}},
		},
		{
			// The image ran 3.3 and standalone 3.1, and this is where the
			// project's owner finds out which one runs now.
			name:  "leftover key, different series",
			deps:  `'apache-airflow==3.1.*'`,
			astro: "airflow = '3.3'\n",
			want: []Problem{{
				CodeAirflowRemoved, "tool.astro.airflow",
				"is no longer read: the Airflow version is the requirement apache-airflow==3.1.* in [project] dependencies. Delete the airflow line. " +
					"It said 3.3, and this project runs 3.1; to run 3.3, change the requirement to apache-airflow==3.3.*.",
			}},
		},
		{
			name:  "leftover key, different series, core",
			deps:  `'apache-airflow-core==3.2.1'`,
			astro: "airflow = '3.3.2'\n",
			want: []Problem{{
				CodeAirflowRemoved, "tool.astro.airflow",
				"is no longer read: the Airflow version is the requirement apache-airflow-core==3.2.1 in [project] dependencies. Delete the airflow line. " +
					"It said 3.3.2, and this project runs 3.2.1; to run 3.3.2, change the requirement to apache-airflow-core==3.3.2.",
			}},
		},
		{
			// A core project is told about its own distribution, not asked to
			// add the full one beside it.
			name:  "leftover key and a core range",
			deps:  `'apache-airflow-core>=3.1'`,
			astro: "airflow = '3.2'\n",
			want: []Problem{
				{
					CodeAirflowUnpinned, "project.dependencies[0]",
					"apache-airflow-core>=3.1 does not pin an Airflow series. Pin one, like apache-airflow-core==3.3.*",
				},
				{
					CodeAirflowRemoved, "tool.astro.airflow",
					"is no longer read: the Airflow version is the apache-airflow-core requirement in [project] dependencies. Delete the airflow line. " +
						"It said 3.2; to keep running 3.2, the requirement is apache-airflow-core==3.2.*.",
				},
			},
		},
		{
			name: "core pinned to an Airflow 2",
			deps: `"apache-airflow-core[otel]==2.10.5 ; os_name == 'posix'"`,
			want: []Problem{{
				CodeAirflowCoreBeforeThree, "project.dependencies[0]",
				"apache-airflow-core[otel]==2.10.5 ; os_name == 'posix' pins an Airflow 2, and apache-airflow-core is published only for Airflow 3. Use apache-airflow==2.10.5",
			}},
		},
		{
			// A core project whose leftover key says an Airflow 2 is told the
			// distribution that exists for it.
			name:  "leftover key naming an Airflow 2 in a core project",
			deps:  `'apache-airflow-core>=3.1'`,
			astro: "airflow = '2.10'\n",
			want: []Problem{
				{
					CodeAirflowUnpinned, "project.dependencies[0]",
					"apache-airflow-core>=3.1 does not pin an Airflow series. Pin one, like apache-airflow-core==3.3.*",
				},
				{
					CodeAirflowRemoved, "tool.astro.airflow",
					"is no longer read: the Airflow version is the apache-airflow-core requirement in [project] dependencies. Delete the airflow line. " +
						"It said 2.10; to keep running 2.10, the requirement is apache-airflow==2.10.*.",
				},
			},
		},
		{
			name:  "leftover key and no requirement",
			deps:  `'pandas'`,
			astro: "airflow = '2.10'\n",
			want: []Problem{
				{CodeAirflowMissing, "project.dependencies", "names no Airflow. Add one, like apache-airflow==3.3.*"},
				{
					CodeAirflowRemoved, "tool.astro.airflow",
					"is no longer read: the Airflow version is the apache-airflow requirement in [project] dependencies. Delete the airflow line. " +
						"It said 2.10; to keep running 2.10, the requirement is apache-airflow==2.10.*.",
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(airflowManifest(tc.deps, tc.astro))
			ve := validationError(t, err)
			if !reflect.DeepEqual(ve.Problems, tc.want) {
				t.Errorf("problems =\n  %#v\nwant\n  %#v", ve.Problems, tc.want)
			}
		})
	}
}

// ParseForRepair loads a manifest whose only fault is the leftover key, so
// the edit that deletes it can run, and refuses everything Parse refuses for
// any other reason.
func TestParseForRepair(t *testing.T) {
	leftover := airflowManifest(`'apache-airflow==3.1.*'`, "airflow = '3.3'\n")
	if _, err := Parse(leftover); err == nil {
		t.Fatal("Parse loaded a manifest carrying [tool.astro] airflow")
	}
	m, err := ParseForRepair(leftover)
	if err != nil {
		t.Fatalf("ParseForRepair refused a manifest whose only problem is the leftover key: %v", err)
	}
	if got := m.Airflow().Pin; got != "3.1" {
		t.Errorf("Airflow().Pin = %q, want the requirement's 3.1, not the key's 3.3", got)
	}

	// The shape a manifest took when the key alone stated the version: no
	// requirement at all. It loads for the repair, which moves the key into
	// one, and still reads as having no Airflow.
	keyOnly, err := ParseForRepair(airflowManifest(`'pandas'`, "airflow = '3.1'\n"))
	if err != nil {
		t.Fatalf("ParseForRepair refused a manifest the key alone pinned: %v", err)
	}
	if got := keyOnly.Airflow(); got != (Airflow{}) {
		t.Errorf("Airflow() = %#v on a manifest with no requirement, want the zero value", got)
	}

	// Every shape a repair of the Airflow version can fix loads, saying what
	// is left to repair: whether the requirement states one version, and what
	// a leftover key says.
	for name, tc := range map[string]struct {
		data    []byte
		unclear bool
		key     string
	}{
		"leftover key":                    {leftover, false, "3.3"},
		"leftover key and no requirement": {airflowManifest(`'pandas'`, "airflow = '3.1'\n"), true, "3.1"},
		"leftover key and a range":        {airflowManifest(`'apache-airflow>=3.1'`, "airflow = '3.1'\n"), true, "3.1"},
		"a range and no key":              {airflowManifest(`'apache-airflow>=3.1'`, ""), true, ""},
		"no requirement and no key":       {airflowManifest(`'pandas'`, ""), true, ""},
		"both distributions":              {airflowManifest(`'apache-airflow==3.3.*', 'apache-airflow-core==3.3.*'`, ""), true, ""},
		"a key that is not a version":     {airflowManifest(`'apache-airflow==3.1.*'`, "airflow = 'latest'\n"), false, ""},
	} {
		m, err := ParseForRepair(tc.data)
		if err != nil {
			t.Errorf("%s: ParseForRepair refused it: %v", name, err)
			continue
		}
		if m.AirflowUnclear() != tc.unclear || m.RemovedAirflowKey() != tc.key {
			t.Errorf("%s: AirflowUnclear() = %v, RemovedAirflowKey() = %q; want %v, %q",
				name, m.AirflowUnclear(), m.RemovedAirflowKey(), tc.unclear, tc.key)
		}
	}

	// Any other problem refuses it, as Parse does.
	_, err = ParseForRepair(airflowManifest(`'apache-airflow>=3.1'`, "airflow = '3.1'\nairflw = '3.1'\n"))
	ve := validationError(t, err)
	if len(ve.Problems) != 3 {
		t.Errorf("want the unknown key reported beside the two Airflow problems, got %v", ve.Problems)
	}

	// A manifest Parse loaded has nothing to repair.
	clean, err := Parse(airflowManifest(`'apache-airflow==3.1.*'`, ""))
	if err != nil {
		t.Fatal(err)
	}
	if clean.AirflowUnclear() || clean.RemovedAirflowKey() != "" {
		t.Errorf("a clean manifest reports something to repair")
	}
}
