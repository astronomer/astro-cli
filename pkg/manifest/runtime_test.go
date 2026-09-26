package manifest

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRuntimeTag(t *testing.T) {
	cases := []struct {
		tag  string
		want RuntimeTag
		ok   bool
	}{
		{tag: "3.3-8", want: RuntimeTag{Major: "3", Series: "3.3", Build: true}, ok: true},
		{tag: "3.10-1", want: RuntimeTag{Major: "3", Series: "3.10", Build: true}, ok: true},
		{tag: "3.3", want: RuntimeTag{Major: "3", Series: "3.3"}, ok: true},
		{tag: "3", want: RuntimeTag{Major: "3"}, ok: true},
		{tag: "13.11.0", want: RuntimeTag{Major: "2", Build: true}, ok: true},
		{tag: "4.0.0", want: RuntimeTag{Major: "2", Build: true}, ok: true},
		{tag: "13", want: RuntimeTag{Major: "2"}, ok: true},
		{tag: "13.1", want: RuntimeTag{Major: "2"}, ok: true},
		// Runtimes 1 to 3 predate every Airflow Docker mode runs, and 3.x.y
		// would read as either generation.
		{tag: "3.0.4", ok: false},
		{tag: "2.1.0", ok: false},
		{tag: "3.3-8-python-3.12", ok: false},
		{tag: "13.7.0-slim", ok: false},
		{tag: "latest", ok: false},
		{tag: "3.3-", ok: false},
		{tag: "", ok: false},
		{tag: "v13.11.0", ok: false},
	}
	for _, tc := range cases {
		got, ok := ParseRuntimeTag(tc.tag)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseRuntimeTag(%q) = %+v, %v; want %+v, %v", tc.tag, got, ok, tc.want, tc.ok)
		}
	}
}

// runtimeManifest is a manifest with one Airflow requirement and a runtime
// line, plus whatever else the case adds to [tool.astro].
func runtimeManifest(requirement, runtime, extra string) string {
	return "[project]\nname = \"p\"\ndependencies = [\"" + requirement + "\"]\n\n[tool.astro]\nruntime = '" + runtime + "'\n" + extra
}

// Guard 1: what the tag alone says about the build is checked against the
// requirement on every load, offline.
func TestRuntimeAgainstTheRequirement(t *testing.T) {
	cases := []struct {
		name        string
		requirement string
		runtime     string
		extra       string
		want        ProblemCode // "" loads
		mention     []string    // substrings the reason names
	}{
		{name: "airflow 3 build of the same series", requirement: "apache-airflow==3.3.*", runtime: "3.3-8"},
		{name: "airflow 3 build under an exact pin of its series", requirement: "apache-airflow==3.3.1", runtime: "3.3-8"},
		{name: "core requirement", requirement: "apache-airflow-core==3.3.*", runtime: "3.3-8"},
		{name: "a pin naming only the generation covers every series", requirement: "apache-airflow==3.*", runtime: "3.2-4"},
		{
			name: "airflow 3 build of another series", requirement: "apache-airflow==3.3.*", runtime: "3.2-10",
			want: CodeRuntimeMismatch, mention: []string{"3.2-10", "Airflow 3.2", "apache-airflow==3.3.*", "Airflow 3.3"},
		},
		{
			name: "another series under an exact pin", requirement: "apache-airflow==3.3.2", runtime: "3.2-10",
			want: CodeRuntimeMismatch, mention: []string{"apache-airflow==3.3.2"},
		},
		// Airflow 2 tags name a runtime version, so only the generation is read.
		{name: "airflow 2 build under an airflow 2 pin", requirement: "apache-airflow==2.11.*", runtime: "13.11.0"},
		{name: "airflow 2 build under another airflow 2 series is the catalog's to catch", requirement: "apache-airflow==2.9.*", runtime: "13.11.0"},
		{
			name: "airflow 2 build under an airflow 3 pin", requirement: "apache-airflow==3.3.*", runtime: "13.11.0",
			want: CodeRuntimeMismatch, mention: []string{"13.11.0", "an Airflow 2", "Airflow 3"},
		},
		{
			name: "airflow 3 build under an airflow 2 pin", requirement: "apache-airflow==2.11.*", runtime: "3.3-8",
			want: CodeRuntimeMismatch, mention: []string{"Airflow 3.3", "Airflow 2"},
		},
		{name: "malformed", requirement: "apache-airflow==3.3.*", runtime: "three", want: CodeRuntimeInvalid, mention: []string{`"three"`}},
		{name: "flavor suffix", requirement: "apache-airflow==3.3.*", runtime: "3.3-8-python-3.12", want: CodeRuntimeInvalid},
		{name: "runtime 3 era tag", requirement: "apache-airflow==3.3.*", runtime: "3.0.4", want: CodeRuntimeInvalid},
		{name: "floating series names no build", requirement: "apache-airflow==3.3.*", runtime: "3.3", want: CodeRuntimeInvalid, mention: []string{"3.3-1"}},
		{name: "floating airflow 2 names no build", requirement: "apache-airflow==2.11.*", runtime: "13", want: CodeRuntimeInvalid},
		{
			name: "beside a dockerfile", requirement: "apache-airflow==3.3.*", runtime: "3.3-8", extra: "dockerfile = 'docker/Dockerfile'\n",
			want: CodeRuntimeWithDockerfile, mention: []string{"docker/Dockerfile", "FROM"},
		},
		{
			name: "beside a dockerfile, whatever the tag", requirement: "apache-airflow==3.3.*", runtime: "3.2-1", extra: "dockerfile = 'Dockerfile'\n",
			want: CodeRuntimeWithDockerfile,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Parse([]byte(runtimeManifest(tc.requirement, tc.runtime, tc.extra)))
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if got := m.Airflow().Runtime; got != tc.runtime {
					t.Errorf("Airflow().Runtime = %q, want %q", got, tc.runtime)
				}
				return
			}
			ve := validationError(t, err)
			if len(ve.Problems) != 1 {
				t.Fatalf("problems = %+v, want one %s", ve.Problems, tc.want)
			}
			p := ve.Problems[0]
			if p.Code != tc.want || p.Key != "tool.astro.runtime" {
				t.Errorf("problem = %s on %s, want %s on tool.astro.runtime", p.Code, p.Key, tc.want)
			}
			for _, s := range tc.mention {
				if !strings.Contains(p.Reason, s) {
					t.Errorf("reason %q does not name %q", p.Reason, s)
				}
			}
		})
	}
}

// An unclear requirement is its own problem, and the runtime is not compared
// against a version nobody can read.
func TestRuntimeIsNotComparedWithAnUnclearRequirement(t *testing.T) {
	_, err := Parse([]byte(runtimeManifest("apache-airflow>=3.1", "3.2-10", "")))
	ve := validationError(t, err)
	for _, p := range ve.Problems {
		if p.Code == CodeRuntimeMismatch {
			t.Errorf("compared against an unpinned requirement: %+v", p)
		}
	}
}

func TestRuntimeMustBeAString(t *testing.T) {
	_, err := Parse([]byte("[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\nruntime = 38\n"))
	ve := validationError(t, err)
	if len(ve.Problems) != 1 || ve.Problems[0].Code != CodeExpectedString || ve.Problems[0].Key != "tool.astro.runtime" {
		t.Errorf("problems = %+v, want expected_string on tool.astro.runtime", ve.Problems)
	}
}

// The edits that move or delete the runtime line read the manifest through
// ParseForRepair, so a runtime problem must not stop it loading there, and
// must not read as an unclear Airflow version.
func TestParseForRepairLetsRuntimeProblemsThrough(t *testing.T) {
	for _, content := range []string{
		runtimeManifest("apache-airflow==3.3.*", "3.2-10", ""),
		runtimeManifest("apache-airflow==3.3.*", "nope", ""),
		runtimeManifest("apache-airflow==3.3.*", "3.3-8", "dockerfile = 'Dockerfile'\n"),
	} {
		m, err := ParseForRepair([]byte(content))
		if err != nil {
			t.Fatalf("ParseForRepair: %v\n%s", err, content)
		}
		if m.AirflowUnclear() {
			t.Errorf("a runtime problem reads as an unclear Airflow version:\n%s", content)
		}
		if got := m.Airflow().Pin; got != "3.3" {
			t.Errorf("Airflow().Pin = %q, want 3.3", got)
		}
		if _, err := Parse([]byte(content)); err == nil {
			t.Errorf("Parse loaded a manifest with a runtime problem:\n%s", content)
		}
	}
}

// dockerfileRuntimeCases are the FROM tags DockerfileRuntimeProblem compares.
// TestEveryCodeIsReachable reads them too.
var dockerfileRuntimeCases = []struct {
	name        string
	requirement string
	tag         string
	want        bool
	mention     []string
}{
	{name: "airflow 3 same series", requirement: "apache-airflow==3.3.*", tag: "3.3-8"},
	{name: "floating tag of the same series", requirement: "apache-airflow==3.3.*", tag: "3.3"},
	{name: "exact pin in the same series", requirement: "apache-airflow==3.3.1", tag: "3.3-8"},
	{
		name: "airflow 3 another series", requirement: "apache-airflow==3.3.*", tag: "3.2-4", want: true,
		mention: []string{"docker/Dockerfile", "runtime:3.2-4", "Airflow 3.2", "apache-airflow==3.3.*", "apache-airflow==3.2.*"},
	},
	{
		name: "floating tag of another series", requirement: "apache-airflow==3.1.*", tag: "3.3", want: true,
		mention: []string{"apache-airflow==3.3.*"},
	},
	{
		name: "core keeps its distribution in the fix", requirement: "apache-airflow-core==3.3.*", tag: "3.2-4", want: true,
		mention: []string{"apache-airflow-core==3.2.*"},
	},
	{
		name: "airflow 2 runtime beside an airflow 3 requirement", requirement: "apache-airflow==3.3.*", tag: "13.11.0", want: true,
		mention: []string{"an Airflow 2", "apache-airflow==2.<minor>.*"},
	},
	{
		name: "airflow 3 runtime beside an airflow 2 requirement", requirement: "apache-airflow==2.11.*", tag: "3.3-8", want: true,
		mention: []string{"Airflow 3.3", "apache-airflow==3.3.*"},
	},
	{name: "airflow 2 runtime beside an airflow 2 requirement", requirement: "apache-airflow==2.11.*", tag: "13.11.0"},
	{name: "an airflow 2 series is not read offline", requirement: "apache-airflow==2.9.*", tag: "13.11.0"},
	{name: "a tag with nothing to read", requirement: "apache-airflow==3.3.*", tag: "latest"},
	{name: "no tag", requirement: "apache-airflow==3.3.*", tag: ""},
}

func dockerfileManifest(t *testing.T, requirement string) *Manifest {
	t.Helper()
	m, err := Parse([]byte("[project]\nname = \"p\"\ndependencies = [\"" + requirement + "\"]\n\n[tool.astro]\ndockerfile = 'docker/Dockerfile'\n"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDockerfileRuntimeProblem(t *testing.T) {
	for _, tc := range dockerfileRuntimeCases {
		t.Run(tc.name, func(t *testing.T) {
			m := dockerfileManifest(t, tc.requirement)
			p, ok := m.DockerfileRuntimeProblem("astrocrpublic.azurecr.io/runtime:"+tc.tag, tc.tag)
			if ok != tc.want {
				t.Fatalf("problem = %v (%+v), want %v", ok, p, tc.want)
			}
			if !ok {
				return
			}
			if p.Code != CodeDockerfileAirflowMismatch || p.Key != "tool.astro.dockerfile" {
				t.Errorf("problem = %s on %s", p.Code, p.Key)
			}
			for _, s := range tc.mention {
				if !strings.Contains(p.Reason, s) {
					t.Errorf("reason %q does not name %q", p.Reason, s)
				}
			}
		})
	}
}

func TestRuntimeProblemsCarryTheirCodeThroughLoad(t *testing.T) {
	_, err := Load(write(t, runtimeManifest("apache-airflow==3.3.*", "3.2-10", "")))
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Path == "" {
		t.Fatalf("Load error = %v, want a ValidationError naming the path", err)
	}
}
