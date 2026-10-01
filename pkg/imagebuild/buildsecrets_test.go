package imagebuild

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
	assert.Equal(t, `building the project's Dockerfile failed; see the build output above: exit status 1 — Dockerfile mounts build secret "netrc", which was not given; pass --build-secret id=netrc,env=<VAR> or set BUILD_SECRET_INPUT`, err.Error())

	two := MissingSecrets{Dockerfile: "Dockerfile", Mounts: []airflowrt.SecretMount{netrc, pip}}
	assert.Equal(t, `building the project's Dockerfile failed; see the build output above: exit status 1 — Dockerfile mounts build secrets "netrc", "pip", which were not given; pass --build-secret id=netrc,env=<VAR> --build-secret id=pip,env=<VAR> or set BUILD_SECRET_INPUT`, two.Explain(buildErr).Error())

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
