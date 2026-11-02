package imagebuild

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func TestGitAuthFailureNamesTheHostAndTheSecret(t *testing.T) {
	base := errors.New("exit status 1")
	for _, tc := range []struct {
		name, line string
		secrets    []string
		want       string
	}{
		{
			name: "no secret",
			line: "#9 0.749          fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			want: "the image's install couldn't log in to github.com to fetch a git dependency. Pass a netrc build secret:\n" +
				"  --build-secret id=netrc,env=NETRC   (or build-secrets = ['id=netrc,env=NETRC'] in pyproject.toml): exit status 1",
		},
		{
			name:    "refused",
			line:    "fatal: Authentication failed for 'https://git.example.com/org/repo.git/'",
			secrets: []string{"id=netrc,env=NETRC"},
			want:    "the image's install couldn't log in to git.example.com to fetch a git dependency: the netrc build secret's credentials for git.example.com were refused: exit status 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g gitAuthHost
			g.scan("#9 0.438    Updating https://github.com/org/repo.git (abc)")
			g.scan(tc.line)
			err := g.explain(base, tc.secrets)
			assert.EqualError(t, err, tc.want)
			assert.ErrorIs(t, err, base)
		})
	}
}

func TestGitAuthLeavesOtherFailuresAlone(t *testing.T) {
	base := errors.New("exit status 1")
	var g gitAuthHost
	g.scan("error: No solution found when resolving dependencies")
	assert.Same(t, base, g.explain(base, nil))
}

// The build's own output is what is read, so a failed install that git could
// not log in for gets the netrc fix in place of the generic error.
func TestBuildNamesTheGitHostItCouldNotLogInTo(t *testing.T) {
	cmd := &fakeCmd{run: func(call string, s rt.Stdio) error {
		if !strings.Contains(call, " build ") {
			return nil
		}
		_, _ = io.WriteString(s.Err, "fatal: could not read Username for 'https://github.com': terminal prompts disabled\n")
		return errors.New("exit status 1")
	}}
	req := testRequest(t)
	req.Dependencies = []string{"corona @ git+https://github.com/org/corona.git"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "couldn't log in to github.com to fetch a git dependency. Pass a netrc build secret")
}
