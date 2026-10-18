package config

import (
	"time"

	"github.com/spf13/afero"
)

// CurrentContextWithoutLogin reads the context GetCurrentContext does, with
// the same errors, and never resolves the login, which GetCurrentContext
// does.
func (s *Suite) TestCurrentContextWithoutLoginNeverResolvesTheLogin() {
	resolved, restore := CountLoginResolutions()
	defer restore()
	defer forgetEnvironmentLogin()

	fs := afero.NewMemMapFs()
	s.Require().NoError(afero.WriteFile(fs, HomeConfigFile, []byte(`context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    refreshtoken: refresh
    organization: org
    organization_product: HOSTED
    workspace: ws
  other_com:
    domain: other.com
  scalar_com: not-a-context
`), 0o600))
	InitConfig(fs)

	// The current context names its entry by key; the domain is the one the
	// entry records, as GetCurrentContext reads it.
	c, err := CurrentContextWithoutLogin()
	s.NoError(err)
	s.Equal(Context{Domain: "example.com", Organization: "org", OrganizationProduct: "HOSTED", Workspace: "ws"}, c)
	s.Zero(resolved(), "CurrentContextWithoutLogin resolved the login")

	full, err := GetCurrentContext()
	s.NoError(err)
	full.Token, full.RefreshToken = "", ""
	s.Equal(full, c, "the same context, but for the login")
	s.Equal(1, resolved(), "the counter sees the resolution GetCurrentContext makes")

	// An environment login's selection is read over the config's, as
	// GetCurrentContext reads it; its token is not.
	s.Require().NoError(full.UseEnvironmentLogin("Bearer env", time.Time{}))
	s.Require().NoError(full.SetEnvironmentContextKey("organization_product", "HYBRID"))
	c, err = CurrentContextWithoutLogin()
	s.NoError(err)
	s.Equal("HYBRID", c.OrganizationProduct)
	s.Empty(c.Token)
	forgetEnvironmentLogin()

	s.T().Setenv("ASTRO_DOMAIN", "other.com")
	c, err = CurrentContextWithoutLogin()
	s.NoError(err)
	s.Equal("other.com", c.Domain)

	s.T().Setenv("ASTRO_DOMAIN", "missing.com")
	_, err = CurrentContextWithoutLogin()
	s.ErrorIs(err, ErrNotConnected)
	_, err = GetCurrentContext()
	s.ErrorIs(err, ErrNotConnected)

	// An entry that is not a context cannot be read, by either.
	s.T().Setenv("ASTRO_DOMAIN", "scalar.com")
	_, err = CurrentContextWithoutLogin()
	s.Error(err)
	_, err = GetCurrentContext()
	s.Error(err)

	s.T().Setenv("ASTRO_DOMAIN", "")
	s.NoError(ResetCurrentContext())
	_, err = CurrentContextWithoutLogin()
	s.ErrorIs(err, ErrGetHomeString)

	s.Equal(1, resolved())
}
