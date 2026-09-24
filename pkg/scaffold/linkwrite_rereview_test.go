package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A re-save with the kind's default method writes no auth table, even when a
// field only another method takes is left over from an earlier choice.
func TestSaveLinkOmitsADefaultAuthTableDespiteAStaleField(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, Link{
		Name: "aws", Kind: manifest.KindMWAA, Environment: "orders",
		Auth: manifest.Auth{Method: manifest.AuthAWS, TokenEnv: "OLD_TOKEN", UsernameEnv: "OLD_USER"},
	})
	assert.NotContains(t, readFile(t, path), "auth")
	_, got := onlyLink(t, path)
	assert.Equal(t, manifest.AuthAWS, got.Auth.Method)

	// A field the method does take still makes the table.
	saveLink(t, dir, endpointLink(&manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN", UsernameEnv: "OLD_USER"}))
	assert.Contains(t, readFile(t, path), "token-env = 'AF_TOKEN'")
}

// A url carrying a username or password is refused, and neither the error nor
// the file ever holds the password, with a scheme or without one.
func TestSaveLinkRefusesAURLWithCredentialsWithoutQuotingIt(t *testing.T) {
	for _, raw := range []string{
		"admin:hunter2@airflow.example.com",
		"https://admin:hunter2@airflow.example.com/api",
		"http://hunter2@airflow.example.com",
		"admin:hunter2@airflow.example.com/path?x=1",
	} {
		dir, path := linkProject(t, linkFixture)
		l := endpointLink(&manifest.Auth{Method: manifest.AuthNone})
		l.URL = raw
		err := SaveLink(dir, nil, l)
		require.ErrorIs(t, err, ErrInvalidLink, raw)
		assert.Contains(t, err.Error(), "must not carry a username or password")
		assert.NotContains(t, err.Error(), "hunter2", raw)
		assert.Equal(t, linkFixture, readFile(t, path))
	}

	// An @ past the host is not userinfo.
	dir, _ := linkProject(t, linkFixture)
	l := endpointLink(&manifest.Auth{Method: manifest.AuthNone})
	l.URL = "https://airflow.example.com/users/@me"
	require.NoError(t, SaveLink(dir, nil, l))
}

// RemoveLink and SetDefaultLink trim the name the way SaveLink does.
func TestRemoveAndDefaultTrimTheName(t *testing.T) {
	body := linkFixture + "workspace = 'W'\n\n[tool.astro.deployments.prod]\ndeployment = 'd1'\n\n[tool.astro.deployments.dev]\ndeployment = 'd2'\n"
	dir, path := linkProject(t, body)
	require.NoError(t, SetDefaultLink(dir, nil, " prod "))
	assert.True(t, loadLinks(t, path).Astro.Deployments["prod"].Default)

	removed, err := RemoveLink(dir, nil, "\tdev ")
	require.NoError(t, err)
	assert.True(t, removed)
	assert.NotContains(t, loadLinks(t, path).Astro.Deployments, "dev")
}
