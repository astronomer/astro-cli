package util

import (
	"errors"
	"testing"
)

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
