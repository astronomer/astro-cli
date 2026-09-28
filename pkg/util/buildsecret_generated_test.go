package util

import (
	"errors"
	"slices"
	"strings"
	"testing"
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

func TestCheckGeneratedBuildSecrets(t *testing.T) {
	if err := CheckGeneratedBuildSecrets([]string{"id=netrc,env=NETRC_CONTENT"}); err != nil {
		t.Errorf("netrc: %v", err)
	}
	if err := CheckGeneratedBuildSecrets([]string{"id=pip,src=/etc/pip.conf"}); !errors.Is(err, ErrBuildSecretNeedsDockerfile) {
		t.Errorf("another id: err = %v, want %v", err, ErrBuildSecretNeedsDockerfile)
	}
	if err := CheckGeneratedBuildSecrets([]string{"id=netrc,env=machine github.com"}); err == nil || errors.Is(err, ErrBuildSecretNeedsDockerfile) {
		t.Errorf("a spec that does not parse: err = %v, want a parse error", err)
	}
}
