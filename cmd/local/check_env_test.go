package local

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
)

// envCheckDeps is targetDeps with a clean parse from both parsers and the
// manifest extended by env, so any finding comes from the declarations.
func envCheckDeps(t *testing.T, env, dotenv string) (Deps, *bytes.Buffer) {
	t.Helper()
	d, out := targetDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest+"\n"+env), 0o600); err != nil {
		t.Fatal(err)
	}
	if dotenv != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	clean := checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}
	d.Checks = stubParser{report: clean}
	d.CheckVenv = stubTargetParser{report: clean}
	return d, out
}

func checkExit(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return checks.ExitOK
	}
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want an exit code, got %v", err)
	}
	return exit.Code
}

// A required value with no source fails the check, the way it blocks
// `astro local start`, and the row names it and the command that provides it.
func TestCheckFailsOnAMissingRequiredValue(t *testing.T) {
	for _, args := range [][]string{{"local", "check"}, {"local", "check", "--target", "astro"}} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			isolateEnvSources(t, "ASTRO_TEST_REQUIRED")
			d, out := envCheckDeps(t, "[tool.astro.env]\nASTRO_TEST_REQUIRED = {}\n[tool.astro.env.connections]\nwarehouse = {}\n", "")
			err := execute(t, d, args...)
			if code := checkExit(t, err); code != checks.ExitChecksFailed {
				t.Fatalf("want exit %d, got %d\n%s", checks.ExitChecksFailed, code, out.String())
			}
			for _, want := range []string{
				"env var ASTRO_TEST_REQUIRED",
				"astro local env variable set ASTRO_TEST_REQUIRED --project",
				"connection warehouse",
				"astro local env connection set warehouse --project",
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output is missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}

// What a start would not refuse over, the check does not fail on: a value
// present, one with a default, one declared optional, and one the workspace
// supplies, which an offline check cannot ask about.
func TestCheckPassesWhatAStartWouldStart(t *testing.T) {
	isolateEnvSources(t, "ASTRO_TEST_SET", "ASTRO_TEST_DEFAULTED", "ASTRO_TEST_OPTIONAL", "ASTRO_TEST_WORKSPACE")
	d, out := envCheckDeps(t, "[tool.astro.env]\n"+
		"ASTRO_TEST_SET = {}\n"+
		"ASTRO_TEST_DEFAULTED = 'x'\n"+
		"ASTRO_TEST_OPTIONAL = { optional = true }\n"+
		"ASTRO_TEST_WORKSPACE = { source = 'workspace' }\n",
		"ASTRO_TEST_SET=yes\n")
	if code := checkExit(t, execute(t, d, "local", "check")); code != checks.ExitOK {
		t.Fatalf("want a pass, got exit %d\n%s", code, out.String())
	}
	if strings.Contains(out.String(), "ASTRO_TEST_") {
		t.Errorf("no declaration should be reported:\n%s", out.String())
	}
}

// A value that is not what its declaration says is a warning, as a start
// prints it: it passes, and --strict fails on it.
func TestCheckWarnsOnAValueOfTheWrongType(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "strict"}[strict], func(t *testing.T) {
			isolateEnvSources(t, "ASTRO_TEST_PORT")
			d, out := envCheckDeps(t, "[tool.astro.env]\nASTRO_TEST_PORT = { type = 'port' }\n", "ASTRO_TEST_PORT=not-a-port\n")
			args := []string{"local", "check"}
			want := checks.ExitOK
			if strict {
				args = append(args, "--strict")
				want = checks.ExitChecksFailed
			}
			if code := checkExit(t, execute(t, d, args...)); code != want {
				t.Fatalf("want exit %d, got %d\n%s", want, code, out.String())
			}
			if !strings.Contains(out.String(), "env var ASTRO_TEST_PORT") {
				t.Errorf("the warning should name the declaration:\n%s", out.String())
			}
		})
	}
}

// The declared environment is this machine's. A platform target sets its own,
// so a value missing here is no finding there.
func TestCheckPlatformTargetIgnoresTheLocalEnvironment(t *testing.T) {
	isolateEnvSources(t, "ASTRO_TEST_REQUIRED")
	d, out := envCheckDeps(t, "[tool.astro.env]\nASTRO_TEST_REQUIRED = {}\n", "")
	if code := checkExit(t, execute(t, d, "local", "check", "--target", "composer")); code != checks.ExitOK {
		t.Fatalf("want a pass, got exit %d\n%s", code, out.String())
	}
}

// In json the finding carries its kind, severity and the declaration it names.
func TestCheckJSONCarriesEnvFindings(t *testing.T) {
	isolateEnvSources(t, "ASTRO_TEST_REQUIRED")
	d, out := envCheckDeps(t, "[tool.astro.env]\nASTRO_TEST_REQUIRED = {}\n", "")
	if code := checkExit(t, execute(t, d, "local", "check", "--output", "json")); code != checks.ExitChecksFailed {
		t.Fatalf("want exit %d, got %d\n%s", checks.ExitChecksFailed, code, out.String())
	}
	var found bool
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var f checks.Finding
		if json.Unmarshal(sc.Bytes(), &f) != nil || f.Kind != checks.KindEnvMissing {
			continue
		}
		found = true
		if f.Severity != checks.SeverityError || f.Section != "env_var" || f.Key != "ASTRO_TEST_REQUIRED" {
			t.Errorf("finding: %+v", f)
		}
	}
	if !found {
		t.Errorf("no env_missing finding in:\n%s", out.String())
	}
}
