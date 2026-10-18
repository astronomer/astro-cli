package local

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

const legacyConfigFile = "/home/me/.astro/config.yaml"

// runningOn is planRecorder with the port of the running Airflow restart
// finds, which a real status always carries.
type runningOn struct {
	planRecorder
	port int
}

func (s runningOn) ReadStatus(dir string) (localrt.Status, error) {
	st, err := s.planRecorder.ReadStatus(dir)
	if st.State == localrt.StateRunning {
		st.Port = s.port
	}
	return st, err
}

const (
	portReason3 = "set to 8099, but it is an Astro CLI 1.x setting v2 does not read. To start on 8099, run astro local start --port 8099"
	portWarning = "warning: " + legacyConfigFile + ": api-server.port: " + portReason3 + "\n"
	secretsNote = "warning: " + legacyConfigFile + ": dev.build_secrets: set, but it is an Astro CLI 1.x setting v2 does not apply. " +
		"Pass --build-secret, or declare build-secrets under [tool.astro] in pyproject.toml\n"
)

type legacyCase struct {
	args     []string
	running  localrt.Mode
	airflow  string // the project's Airflow pin; 3.1 when empty
	astro    string // the project's [tool.astro]
	kept     int    // a port the project kept from an earlier start
	settings map[string]string
}

// A start names --port for the 1.x port setting of the project's Airflow,
// unless it has a port of its own: a --port, one the project kept from an
// earlier start, or a running Airflow's, which a restart keeps. A restart
// with nothing running is a start, and is told.
func TestStartWarnsOf1xPort(t *testing.T) {
	both := map[string]string{legacyAPIServerPort: "8099", legacyWebserverPort: "8100"}
	for _, tc := range []struct {
		name string
		legacyCase
		want string
	}{
		{name: "airflow 3", legacyCase: legacyCase{args: []string{"local", "start"}, settings: both}, want: portWarning},
		{
			name:       "airflow 2",
			legacyCase: legacyCase{args: []string{"local", "start"}, airflow: "2.10", settings: both},
			want: "warning: " + legacyConfigFile + ": webserver.port: set to 8100, but it is an Astro CLI 1.x setting v2 does not read. " +
				"To start on 8100, run astro local start --port 8100\n",
		},
		{name: "the other generation's key", legacyCase: legacyCase{args: []string{"local", "start"}, settings: map[string]string{legacyWebserverPort: "8100"}}},
		{name: "unset or default", legacyCase: legacyCase{args: []string{"local", "start"}, settings: map[string]string{}}},
		{name: "zero, which 1.x read as unset", legacyCase: legacyCase{args: []string{"local", "start"}, settings: map[string]string{legacyAPIServerPort: "0"}}},
		{
			name:       "not a port",
			legacyCase: legacyCase{args: []string{"local", "start"}, settings: map[string]string{legacyAPIServerPort: "99999"}},
			want: "warning: " + legacyConfigFile + `: api-server.port: set to "99999", which is not a valid port, and it is an Astro CLI 1.x setting v2 does not read. ` +
				"Pick the port with astro local start --port <port>\n",
		},
		{name: "overridden by --port", legacyCase: legacyCase{args: []string{"local", "start", "--port", "8099"}, settings: both}},
		{name: "a port the project kept", legacyCase: legacyCase{args: []string{"local", "start"}, kept: 8099, settings: both}},
		{name: "restart keeps the running port", legacyCase: legacyCase{args: []string{"local", "restart"}, running: localrt.ModeStandalone, settings: both}},
		{name: "restart of nothing running", legacyCase: legacyCase{args: []string{"local", "restart"}, settings: both}, want: portWarning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := runWithLegacySettings(t, tc.legacyCase)
			assertLegacyWarning(t, stdout, stderr, tc.want)
		})
	}
}

// A Docker-mode start of a project's own Dockerfile names --build-secret for
// a 1.x dev.build_secrets, unless the start has build secrets of its own. No
// other start takes build secrets, so no other start is told.
func TestStartWarnsOf1xBuildSecrets(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	set := map[string]string{legacyBuildSecrets: "id=netrc,env=NETRC_CONTENT"}
	docker := []string{"local", "start", "--docker", "--port", "8080"}
	for _, tc := range []struct {
		name string
		legacyCase
		env  string
		want string
	}{
		{name: "set", legacyCase: legacyCase{args: docker, astro: declaredDockerfile, settings: set}, want: secretsNote},
		{name: "unset", legacyCase: legacyCase{args: docker, astro: declaredDockerfile, settings: map[string]string{}}},
		{name: "restart", legacyCase: legacyCase{args: []string{"local", "restart"}, running: localrt.ModeDocker, astro: declaredDockerfile, settings: set}, want: secretsNote},
		{
			name:       "overridden by --build-secret",
			legacyCase: legacyCase{args: append(docker, "--build-secret", "id=netrc,env=NETRC_CONTENT"), astro: declaredDockerfile, settings: set},
		},
		{name: "overridden by BUILD_SECRET_INPUT", legacyCase: legacyCase{args: docker, astro: declaredDockerfile, settings: set}, env: "id=netrc,env=NETRC_CONTENT"},
		{
			name:       "overridden by the manifest",
			legacyCase: legacyCase{args: docker, astro: declaredDockerfile + "build-secrets = ['id=netrc,env=NETRC_CONTENT']\n", settings: set},
		},
		{name: "generated build", legacyCase: legacyCase{args: docker, settings: set}},
		{name: "standalone", legacyCase: legacyCase{args: []string{"local", "start", "--port", "8080"}, astro: declaredDockerfile, settings: set}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, tc.env)
			stdout, stderr := runWithLegacySettings(t, tc.legacyCase)
			assertLegacyWarning(t, stdout, stderr, tc.want)
		})
	}
}

// In json mode the warning is an event like every other start warning, with
// the setting as its key, so Astro Desktop need not parse the prose.
func TestThe1xWarningIsAJSONEvent(t *testing.T) {
	stdout, _ := runWithLegacySettings(t, legacyCase{
		args:     []string{"local", "start", "--output", "json"},
		settings: map[string]string{legacyAPIServerPort: "8099"},
	})
	var found bool
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		var e event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("stdout line is not json: %q: %v", sc.Text(), err)
		}
		if e.Key == legacyAPIServerPort {
			found = true
			if e.Event != "warning" || e.Reason != portReason3 {
				t.Errorf("event = %+v, want a warning with reason %q", e, portReason3)
			}
		}
	}
	if !found {
		t.Errorf("no warning event for %s in %q", legacyAPIServerPort, stdout)
	}
}

// The seam is asked about the project being started, which is where a 1.x
// project's own .astro/config.yaml is.
func TestThe1xSettingIsReadForTheProject(t *testing.T) {
	d, _ := testDeps(t)
	var plans []localrt.Plan
	d.Runtime = planRecorder{plans: &plans}
	wiringProject(t, &d, "", "")
	want, _ := d.WorkingDir()
	var asked []string
	d.Legacy1xSetting = func(dir, _ string) (string, string) {
		asked = append(asked, dir)
		return "", ""
	}
	_ = execute(t, d, "local", "start")
	if len(asked) == 0 {
		t.Fatal("the seam was never asked")
	}
	for _, dir := range asked {
		if dir != want {
			t.Errorf("asked about %q, want the project %q", dir, want)
		}
	}
}

// Without the seam, as in a build that does not wire it, a start is not told.
func TestStartWithoutLegacySettingsReaderSaysNothing(t *testing.T) {
	stdout, stderr := runWithLegacySettings(t, legacyCase{args: []string{"local", "start"}})
	assertLegacyWarning(t, stdout, stderr, "")
}

// runWithLegacySettings runs tc in a project of its own, with a config
// holding tc.settings in legacyConfigFile. nil settings leave the seam
// unwired. The start itself fails in the fake runtime; the warnings come
// before it.
func runWithLegacySettings(t *testing.T, tc legacyCase) (stdout, stderr string) {
	t.Helper()
	d, out := testDeps(t)
	errOut := &bytes.Buffer{}
	d.Stderr = errOut
	var plans []localrt.Plan
	d.Runtime = runningOn{planRecorder: planRecorder{plans: &plans, mode: tc.running}, port: 8081}
	from := ""
	if strings.Contains(tc.astro, "dockerfile") {
		from = matchingBase
	}
	wiringProject(t, &d, tc.astro, from)
	dir, _ := d.WorkingDir()
	if tc.airflow != "" {
		m := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==" + tc.airflow + ".*']\n\n[tool.astro]\n" + tc.astro
		if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if tc.kept > 0 {
		if err := plan.PersistPort(dir, tc.kept); err != nil {
			t.Fatal(err)
		}
	}
	if tc.settings != nil {
		d.Legacy1xSetting = func(_, key string) (string, string) { return tc.settings[key], legacyConfigFile }
	}
	_ = execute(t, d, tc.args...)
	if len(plans) != 1 {
		t.Fatalf("started %d times, want 1; stdout was %q, stderr %q", len(plans), out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

// assertLegacyWarning holds the start's output to carrying want, once, or no
// 1.x warning when want is empty.
func assertLegacyWarning(t *testing.T, stdout, stderr, want string) {
	t.Helper()
	if strings.Contains(stderr, "Astro CLI 1.x") {
		t.Errorf("a 1.x warning reached stderr rather than the start's own warnings: %q", stderr)
	}
	if want == "" {
		if strings.Contains(stdout, "Astro CLI 1.x") {
			t.Errorf("output carries a 1.x warning it should not: %q", stdout)
		}
		return
	}
	if !strings.Contains(stdout, want) {
		t.Errorf("output = %q, want it to carry %q", stdout, want)
	}
	if n := strings.Count(stdout, "Astro CLI 1.x"); n != 1 {
		t.Errorf("output carries %d 1.x warnings, want 1: %q", n, stdout)
	}
}
