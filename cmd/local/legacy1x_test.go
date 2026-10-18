package local

import (
	"bytes"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/util"
)

const legacyConfigFile = "/home/me/.astro/config.yaml"

// legacySettings is a Deps.Legacy1xSetting over a fixed home config. A key
// missing from it reads as unset or 1.x's default, which the config side
// (config.Legacy1xHomeValue) tells apart from a choice.
func legacySettings(values map[string]string) func(string) (string, string) {
	return func(key string) (string, string) { return values[key], legacyConfigFile }
}

const (
	portNote = "note: webserver.port is 8099 in " + legacyConfigFile +
		", an Astro CLI 1.x setting v2 does not read. To start on 8099, run astro local start --port 8099\n"
	buildSecretsNote = "note: dev.build_secrets is set in " + legacyConfigFile +
		", an Astro CLI 1.x setting v2 does not apply. Pass --build-secret, or declare build-secrets under [tool.astro] in pyproject.toml\n"
)

// A start names --port for a 1.x port it no longer reads, unless it was
// given one: a --port, or a restart of a running Airflow, which keeps its own.
// A restart with nothing running is a start, and is told.
func TestStartHintsAt1xPort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		running  localrt.Mode
		settings map[string]string
		want     string
	}{
		{name: "set", args: []string{"local", "start"}, settings: map[string]string{legacyWebserverPort: "8099"}, want: portNote},
		{name: "json", args: []string{"local", "start", "--output", "json"}, settings: map[string]string{legacyWebserverPort: "8099"}, want: portNote},
		{name: "unset or default", args: []string{"local", "start"}},
		{name: "overridden by --port", args: []string{"local", "start", "--port", "8099"}, settings: map[string]string{legacyWebserverPort: "8099"}},
		{name: "restart keeps the running port", args: []string{"local", "restart"}, running: localrt.ModeStandalone, settings: map[string]string{legacyWebserverPort: "8099"}},
		{name: "restart of nothing running", args: []string{"local", "restart"}, settings: map[string]string{legacyWebserverPort: "8099"}, want: portNote},
		{
			name:     "one note for both keys",
			args:     []string{"local", "start"},
			settings: map[string]string{legacyAPIServerPort: "9090", legacyWebserverPort: "8099"},
			want: "note: api-server.port is 9090 in " + legacyConfigFile +
				", an Astro CLI 1.x setting v2 does not read. To start on 9090, run astro local start --port 9090\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := runWithLegacySettings(t, tc.args, tc.running, "", tc.settings)
			assertLegacyNote(t, stdout, stderr, tc.want)
		})
	}
}

// A Docker-mode start of a project's own Dockerfile names --build-secret for
// a 1.x dev.build_secrets, unless the start has build secrets of its own. No
// other start takes build secrets, so no other start is told.
func TestStartHintsAt1xBuildSecrets(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	set := map[string]string{legacyBuildSecrets: "id=netrc,env=NETRC_CONTENT"}
	for _, tc := range []struct {
		name     string
		args     []string
		running  localrt.Mode
		astro    string
		env      string
		settings map[string]string
		want     string
	}{
		{name: "set", args: []string{"local", "start", "--docker"}, astro: declaredDockerfile, settings: set, want: buildSecretsNote},
		{name: "unset", args: []string{"local", "start", "--docker"}, astro: declaredDockerfile},
		{name: "restart", args: []string{"local", "restart"}, running: localrt.ModeDocker, astro: declaredDockerfile, settings: set, want: buildSecretsNote},
		{
			name:  "overridden by --build-secret",
			args:  []string{"local", "start", "--docker", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			astro: declaredDockerfile, settings: set,
		},
		{name: "overridden by BUILD_SECRET_INPUT", args: []string{"local", "start", "--docker"}, astro: declaredDockerfile, env: "id=netrc,env=NETRC_CONTENT", settings: set},
		{
			name:  "overridden by the manifest",
			args:  []string{"local", "start", "--docker"},
			astro: declaredDockerfile + "build-secrets = ['id=netrc,env=NETRC_CONTENT']\n", settings: set,
		},
		{name: "generated build", args: []string{"local", "start", "--docker"}, settings: set},
		{name: "standalone", args: []string{"local", "start"}, astro: declaredDockerfile, settings: set},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(util.BuildSecretInputEnv, tc.env)
			stdout, stderr := runWithLegacySettings(t, tc.args, tc.running, tc.astro, tc.settings)
			assertLegacyNote(t, stdout, stderr, tc.want)
		})
	}
}

// Without the seam, as in a build that does not wire it, a start is not told.
func TestStartWithoutLegacySettingsReaderSaysNothing(t *testing.T) {
	stdout, stderr := runWithLegacySettings(t, []string{"local", "start"}, "", "", nil)
	assertLegacyNote(t, stdout, stderr, "")
}

// runWithLegacySettings runs args in a project whose [tool.astro] is astro,
// with running naming the mode of an Airflow restart finds (none when empty),
// and a home config holding settings. nil settings leave the seam unwired.
// The start itself fails in the fake runtime; the notes come before it.
func runWithLegacySettings(t *testing.T, args []string, running localrt.Mode, astro string, settings map[string]string) (stdout, stderr string) {
	t.Helper()
	d, out := testDeps(t)
	errOut := &bytes.Buffer{}
	d.Stderr = errOut
	var plans []localrt.Plan
	d.Runtime = planRecorder{plans: &plans, mode: running}
	from := ""
	if strings.Contains(astro, "dockerfile") {
		from = matchingBase
	}
	wiringProject(t, &d, astro, from)
	if settings != nil {
		d.Legacy1xSetting = legacySettings(settings)
	}
	_ = execute(t, d, args...)
	if len(plans) != 1 {
		t.Fatalf("started %d times, want 1; stderr was %q", len(plans), errOut.String())
	}
	return out.String(), errOut.String()
}

// assertLegacyNote holds stderr to carrying want, or no 1.x note when want is
// empty, and stdout to carrying none either way.
func assertLegacyNote(t *testing.T, stdout, stderr, want string) {
	t.Helper()
	if strings.Contains(stdout, "Astro CLI 1.x") {
		t.Errorf("a 1.x note reached stdout: %q", stdout)
	}
	if want == "" {
		if strings.Contains(stderr, "Astro CLI 1.x") {
			t.Errorf("stderr carries a 1.x note it should not: %q", stderr)
		}
		return
	}
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to carry %q", stderr, want)
	}
	if n := strings.Count(stderr, "Astro CLI 1.x"); n != 1 {
		t.Errorf("stderr carries %d 1.x notes, want 1: %q", n, stderr)
	}
}
