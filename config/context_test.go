package config

import (
	"bytes"
	"time"

	"github.com/spf13/afero"
)

var err error

func (s *Suite) TestGetCurrentContextError() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://example.com:8871/v1
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)
	_, err = GetCurrentContext()
	s.EqualError(err, "no context set, have you authenticated to Astro or APC? Run astro login and try again")
}

func (s *Suite) TestPrintContext() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)

	ctx := Context{
		Token:             "token",
		LastUsedWorkspace: "ck05r3bor07h40d02y2hw4n4v",
		Workspace:         "ck05r3bor07h40d02y2hw4n4v",
		Domain:            "example.com",
	}
	buf := new(bytes.Buffer)
	err = ctx.PrintCloudContext(buf)
	s.NoError(err)
	expected := " CONTROLPLANE                        WORKSPACE                           \n example.com                         ck05r3bor07h40d02y2hw4n4v           \n"
	s.Equal(expected, buf.String())
}

func (s *Suite) TestPrintContextNA() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)

	ctx := Context{
		Token:             "token",
		LastUsedWorkspace: "ck05r3bor07h40d02y2hw4n4v",
		Workspace:         "",
		Domain:            "example.com",
	}
	buf := new(bytes.Buffer)
	err = ctx.PrintCloudContext(buf)
	s.NoError(err)
	expected := " CONTROLPLANE                        WORKSPACE                           \n example.com                         N/A                                 \n"
	s.Equal(expected, buf.String())
}

func (s *Suite) TestGetCurrentContext() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)
	ctx, err := GetCurrentContext()
	s.NoError(err)
	s.Equal("example.com", ctx.Domain)
	s.Equal("token", ctx.Token)
	s.Equal("ck05r3bor07h40d02y2hw4n4v", ctx.Workspace)
}

func (s *Suite) TestGetCurrentContext_WithDomainOverride() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
  stage_example_com:
    domain: stage.example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4w
    workspace: ck05r3bor07h40d02y2hw4n4w
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)
	s.T().Setenv("ASTRO_DOMAIN", "stage.example.com")
	ctx, err := GetCurrentContext()
	s.NoError(err)
	s.Equal("stage.example.com", ctx.Domain)
	s.Equal("token", ctx.Token)
	s.Equal("ck05r3bor07h40d02y2hw4n4w", ctx.Workspace)
}

func (s *Suite) TestDeleteContext() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: test_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
    organization: test-org-id
  test_com:
    domain: test.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
    organization: test-org-id
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)
	ctx := Context{Domain: "exmaple.com"}
	err := ctx.DeleteContext()
	s.NoError(err)

	ctx = Context{}
	err = ctx.DeleteContext()
	s.ErrorIs(err, ErrCtxConfigErr)
}

func (s *Suite) TestResetCurrentContext() {
	initTestConfig()
	err := ResetCurrentContext()
	s.NoError(err)
	ctx, err := GetCurrentContext()
	s.Equal("", ctx.Domain)
	s.ErrorIs(err, ErrGetHomeString)
}

func (s *Suite) TestGetContexts() {
	initTestConfig()
	ctxs, err := GetContexts()
	s.NoError(err)
	context := func(domain string) Context {
		return Context{Domain: domain, Organization: "test-org-id", Workspace: "ck05r3bor07h40d02y2hw4n4v", LastUsedWorkspace: "ck05r3bor07h40d02y2hw4n4v", Token: "token"}
	}
	s.Equal(Contexts{Contexts: map[string]Context{"test_com": context("test.com"), "example_com": context("example.com")}}, ctxs)
}

func (s *Suite) TestSetContextKey() {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContextKey("token", "test")
	outCtx, err := ctx.GetContext()
	s.NoError(err)
	s.Equal("test", outCtx.Token)
}

// Regression: viper's Set on a nested path populates its override layer with
// a partial tree, causing UnmarshalKey of the parent to drop any field not
// touched by a Set (e.g. refreshtoken). See viper#1106.
func (s *Suite) TestSetContextKey_PreservesSiblingFields() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: example.com
contexts:
  example_com:
    domain: example.com
    token: Bearer old
    refreshtoken: original-refresh-token
    organization: org-id
    workspace: ws-id
`)
	err = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	s.Require().NoError(err)
	InitConfig(fs)

	ctx, err := GetCurrentContext()
	s.Require().NoError(err)
	s.Require().NoError(ctx.SetContextKey("token", "Bearer new"))
	s.Require().NoError(ctx.SetExpiresIn(3600))

	reread, err := GetCurrentContext()
	s.Require().NoError(err)
	s.Equal("Bearer new", reread.Token)
	s.Equal("original-refresh-token", reread.RefreshToken)
	s.Equal("org-id", reread.Organization)
	s.Equal("ws-id", reread.Workspace)
	s.Equal("example.com", reread.Domain)
}

func (s *Suite) TestSetSharedContextKey_SharesLoginAcrossTenant() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: pr1111.astronomer-dev.io
contexts:
  pr1111_astronomer-dev_io:
    domain: pr1111.astronomer-dev.io
    auth_domain: https://sandbox.example.com/
    auth_client_id: sandbox-client
    workspace: ws-1111
    expiresin: 2000-01-01T00:00:00Z
  pr2222_astronomer-dev_io:
    domain: pr2222.astronomer-dev.io
    auth_domain: https://sandbox.example.com/
    auth_client_id: sandbox-client
    token: Bearer old
    workspace: ws-2222
  astronomer-dev_io:
    domain: astronomer-dev.io
    auth_domain: https://auth.astronomer-dev.io/
    auth_client_id: dev-client
    token: Bearer dev
  pr3333_astronomer-dev_io:
    domain: pr3333.astronomer-dev.io
    token: Bearer never-recorded
`)
	s.Require().NoError(afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777))
	InitConfig(fs)

	ctx := Context{Domain: "pr1111.astronomer-dev.io"}
	s.Require().NoError(ctx.SetSharedContextKey("token", "Bearer shared"))
	s.Require().NoError(ctx.SetSharedContextKey("refreshtoken", "shared-refresh"))
	s.Require().NoError(ctx.SetSharedExpiresIn(3600))
	s.Require().NoError(ctx.SetContextKey("workspace", "ws-new"))

	InitConfig(fs)
	contexts, err := GetContexts()
	s.Require().NoError(err)
	sibling := contexts.Contexts["pr2222_astronomer-dev_io"]
	s.Equal("Bearer shared", sibling.Token)
	s.Equal("shared-refresh", sibling.RefreshToken)
	s.Equal("ws-2222", sibling.Workspace)
	siblingExpiry, err := sibling.GetExpiresIn()
	s.NoError(err)
	ownExpiry, err := ctx.GetExpiresIn()
	s.NoError(err)
	s.Equal(ownExpiry, siblingExpiry)

	s.Equal("Bearer dev", contexts.Contexts["astronomer-dev_io"].Token)
	s.Equal("Bearer never-recorded", contexts.Contexts["pr3333_astronomer-dev_io"].Token)
	s.Equal("ws-new", contexts.Contexts["pr1111_astronomer-dev_io"].Workspace)

	s.Run("a new expiry replaces the old one every time", func() {
		for i := range 20 {
			set := ctx.SetExpiresIn
			if i%2 == 0 {
				set = ctx.SetSharedExpiresIn
			}
			s.Require().NoError(set(3600))
			expiry, err := ctx.GetExpiresIn()
			s.NoError(err)
			s.True(expiry.After(time.Now()), "expiry %s is in the past", expiry)
		}
	})

	s.Run("SetContextKey keeps a login field on its own context", func() {
		s.Require().NoError(ctx.SetContextKey("token", "Bearer api-token"))
		s.Require().NoError(ctx.SetExpiresIn(3600))
		s.Require().NoError(ctx.SetContextKey("refreshtoken", ""))
		pr1111, err := ctx.GetContext()
		s.NoError(err)
		s.Equal("Bearer api-token", pr1111.Token)
		sibling, err := (&Context{Domain: "pr2222.astronomer-dev.io"}).GetContext()
		s.NoError(err)
		s.Equal("Bearer shared", sibling.Token)
		s.Equal("shared-refresh", sibling.RefreshToken)
	})

	s.Run("only a login field is shared", func() {
		s.ErrorIs(ctx.SetSharedContextKey("workspace", "ws-shared"), errNotLoginField)
		sibling, err := (&Context{Domain: "pr2222.astronomer-dev.io"}).GetContext()
		s.NoError(err)
		s.Equal("ws-2222", sibling.Workspace)
	})
}

func (s *Suite) TestContextsSharingLogin() {
	initTestConfig()
	s.Require().NoError((&Context{Domain: "pr1111.astronomer-dev.io"}).SetContext())
	s.Require().NoError((&Context{Domain: "pr2222.astronomer-dev.io"}).SetContext())
	s.Require().NoError((&Context{Domain: "pr1111.astronomer-dev.io"}).SetAuthTenant("https://sandbox.example.com/", "sandbox-client"))
	s.Require().NoError((&Context{Domain: "pr2222.astronomer-dev.io"}).SetAuthTenant("https://sandbox.example.com/", "other-client"))

	sharing, err := ContextsSharingLogin("https://sandbox.example.com/", "sandbox-client")
	s.NoError(err)
	s.Len(sharing, 1)
	s.Equal("pr1111.astronomer-dev.io", sharing[0].Domain)

	sharing, err = ContextsSharingLogin("", "")
	s.NoError(err)
	s.Empty(sharing)

	s.Run("a context saved without its domain field is named by its key", func() {
		fs := afero.NewMemMapFs()
		configRaw := []byte(`
contexts:
  pr1111_astronomer-dev_io:
    auth_domain: https://sandbox.example.com/
    auth_client_id: sandbox-client
`)
		s.Require().NoError(afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777))
		InitConfig(fs)

		sharing, err := ContextsSharingLogin("https://sandbox.example.com/", "sandbox-client")
		s.NoError(err)
		s.Require().Len(sharing, 1)
		s.Equal("pr1111.astronomer-dev.io", sharing[0].Domain)
	})
}

func (s *Suite) TestSetContextKey_KeepsDomainOfNewContext() {
	initTestConfig()
	ctx := Context{Domain: "pr1111.astronomer-dev.io"}
	s.Require().NoError(ctx.SetContext())
	s.Require().NoError(ctx.SetContextKey("token", "Bearer new"))

	contexts, err := GetContexts()
	s.Require().NoError(err)
	s.Equal("pr1111.astronomer-dev.io", contexts.Contexts["pr1111_astronomer-dev_io"].Domain)
}

func (s *Suite) TestSetOrganizationContext() {
	initTestConfig()
	s.Run("set organization context", func() {
		ctx := Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "HYBRID")
		outCtx, err := ctx.GetContext()
		s.NoError(err)
		s.Equal("org1", outCtx.Organization)
		s.Equal("HYBRID", outCtx.OrganizationProduct)
	})

	s.Run("set organization context error", func() {
		ctx := Context{Domain: ""}
		s.NoError(err)
		err = ctx.SetOrganizationContext("org1", "HYBRID")
		s.Error(err)
		s.Contains(err.Error(), "context config invalid, no domain specified")
	})
}

func (s *Suite) TestExpiresIn() {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	err := ctx.SetExpiresIn(12)
	s.NoError(err)

	outCtx, err := ctx.GetContext()
	s.NoError(err)

	val, err := outCtx.GetExpiresIn()
	s.NoError(err)
	s.Equal("localhost", outCtx.Domain)
	s.True(time.Now().Add(time.Duration(12) * time.Second).After(val)) // now + 12 seconds will always be after expire time, since that is set before
}

func (s *Suite) TestExpiresInFailure() {
	initTestConfig()
	ctx := Context{}
	err := ctx.SetExpiresIn(1)
	s.ErrorIs(err, ErrCtxConfigErr)

	val, err := ctx.GetExpiresIn()
	s.ErrorIs(err, ErrCtxConfigErr)
	s.Equal(time.Time{}, val)
}
