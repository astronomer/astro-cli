package imagebuild

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

func TestCheckBuildSecretsForAGeneratedBuild(t *testing.T) {
	t.Setenv("NETRC_SET", "machine github.com")
	for _, tc := range []struct {
		name     string
		secrets  []string
		wantErr  string
		wantWarn []string
	}{
		{name: "netrc with its variable set", secrets: []string{"id=netrc,env=NETRC_SET"}},
		{name: "netrc with its variable unset", secrets: []string{"id=netrc,env=NETRC_UNSET_FOR_TEST"}, wantErr: "NETRC_UNSET_FOR_TEST, which is empty or not set"},
		{name: "a spec that does not parse", secrets: []string{"id=netrc,env=machine github.com"}, wantErr: "not an environment variable name"},
		{
			name:     "another id is dropped with a warning",
			secrets:  []string{"id=pip,src=/etc/pip.conf", "id=netrc,env=NETRC_SET"},
			wantWarn: []string{`build secret "pip" is not used: an image built without a Dockerfile reads only the netrc secret`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missing, err := CheckBuildSecrets(t.TempDir(), "", tc.secrets)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if got := missing.Warnings(); !slices.Equal(got, tc.wantWarn) {
				t.Errorf("warnings = %q, want %q", got, tc.wantWarn)
			}
		})
	}
}

func TestMissingSecretsExplain(t *testing.T) {
	buildErr := fmt.Errorf("%w: exit status 1", ErrDockerfileBuild)
	netrc := airflowrt.SecretMount{ID: "netrc", Line: 5}
	pip := airflowrt.SecretMount{ID: "pip", Line: 9}

	one := MissingSecrets{Dockerfile: "Dockerfile", Mounts: []airflowrt.SecretMount{netrc}}
	err := one.Explain(buildErr)
	assert.ErrorIs(t, err, ErrDockerfileBuild)
	assert.Equal(t, `building the project's Dockerfile failed; see the build output above: exit status 1. Dockerfile mounts build secret "netrc", which was not given`, err.Error())

	two := MissingSecrets{Dockerfile: "Dockerfile", Mounts: []airflowrt.SecretMount{netrc, pip}}
	assert.Equal(t, `building the project's Dockerfile failed; see the build output above: exit status 1. Dockerfile mounts build secrets "netrc", "pip", which were not given`, two.Explain(buildErr).Error())

	two.Hint = func(ids []string) string { return "give " + strings.Join(ids, " and ") }
	assert.Equal(t, `building the project's Dockerfile failed; see the build output above: exit status 1. Dockerfile mounts build secrets "netrc", "pip", which were not given; give netrc and pip`, two.Explain(buildErr).Error())

	other := errors.New("docker is not running")
	assert.Equal(t, other, one.Explain(other), "only a failed Dockerfile build gets the hint")
	assert.Equal(t, buildErr, MissingSecrets{}.Explain(buildErr), "nothing missing, nothing added")
	assert.NoError(t, one.Explain(nil))
}

func TestCheckBuildSecretEnv(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	t.Setenv("PIP_CONF", "")
	assert.NoError(t, checkBuildSecretEnv([]string{"id=netrc,env=NETRC_CONTENT", "id=ca,src=/etc/ca.pem", "id=netrc"}))

	err := checkBuildSecretEnv([]string{"id=netrc,env=NETRC_CONTENT", "id=pip,type=env,src=PIP_CONF"})
	assert.EqualError(t, err, `build secret "pip" reads the environment variable PIP_CONF, which is empty or not set. Set it before the build, or give the secret another source`)

	err = checkBuildSecretEnv([]string{"id=netrc,env=machine github.com password hunter2"})
	assert.ErrorIs(t, err, manifest.ErrBuildSecretEnvNotAName)
	assert.NotContains(t, err.Error(), "hunter2")

	err = checkBuildSecretEnv([]string{"id=gh,env=ghp_hunter2Token"})
	assert.EqualError(t, err, `build secret "gh" reads the environment variable its env= names, which is empty or not set. Set it before the build, or give the secret another source`)

	err = checkBuildSecretEnv([]string{"id=pw,env=hunter2,def"})
	assert.ErrorContains(t, err, "not a key=value pair")
	assert.NotContains(t, err.Error(), "hunter2")
}

// A mount that says required=false, as the install step's netrc mount does, is
// not warned about on every build; a mount of the project's own still
// is, and a failed build names only that one: the build ran the install step
// without netrc, so its absence is not why it failed.
func TestMissingBuildSecretsLeavesTheInstallStepsMountOutOfTheWarnings(t *testing.T) {
	dir := t.TempDir()
	body := airflowrt.InstallBaseDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", true) +
		"RUN --mount=type=secret,id=pip,target=/etc/pip.conf pip install foo\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(body), 0o600))

	m := MissingBuildSecrets(dir, "Dockerfile", nil)
	require.Len(t, m.Mounts, 2)
	warnings := m.Warnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `build secret "pip"`)
	explained := m.Explain(fmt.Errorf("%w: exit status 1", ErrDockerfileBuild)).Error()
	assert.Contains(t, explained, `build secret "pip", which was not given`)
	assert.NotContains(t, explained, "netrc")
}

// A failed build of a Dockerfile whose only missing mount is optional comes
// back as it is: nothing blames the secret.
func TestExplainLeavesAFailureAloneWhenOnlyAnOptionalMountIsMissing(t *testing.T) {
	m := MissingSecrets{Dockerfile: "Dockerfile", Mounts: []airflowrt.SecretMount{{ID: "netrc", Line: 9, Optional: true}}}
	failed := fmt.Errorf("%w: exit status 1", ErrDockerfileBuild)
	assert.Equal(t, failed, m.Explain(failed))
}

// The install step mounts the secret the manifest names for a private index.
func TestInstallBaseMountsTheRuntimeSecret(t *testing.T) {
	assert.Contains(t, airflowrt.InstallBaseDockerfile("x", true), "--mount=type=secret,id="+manifest.RuntimeSecretID+",")
}

func TestMissingSecretsNameNoToolWithoutAHint(t *testing.T) {
	m := MissingSecrets{Dockerfile: "Dockerfile", Mounts: []airflowrt.SecretMount{{ID: "netrc", Line: 2}}}
	assert.Equal(t, []string{`Dockerfile mounts build secret "netrc" (line 2) but none was given`}, m.Warnings())
	explained := m.Explain(fmt.Errorf("%w: exit status 1", ErrDockerfileBuild)).Error()
	for _, text := range append(m.Warnings(), explained) {
		assert.NotContains(t, text, "--", "a flag is the caller's to name")
		assert.NotContains(t, text, "BUILD_SECRET_INPUT", "a variable is the caller's to name")
		assert.NotContains(t, text, "—", "the desktop shows this text, and its copy has no em-dashes")
	}

	m.Hint = func(ids []string) string { return "give " + strings.Join(ids, " ") }
	assert.Equal(t, []string{`Dockerfile mounts build secret "netrc" (line 2) but none was given; give netrc`}, m.Warnings())
}
