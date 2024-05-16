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
	s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
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
	s.Equal(Contexts{Contexts: map[string]Context{"test_com": {"test.com", "test-org-id", "", "ck05r3bor07h40d02y2hw4n4v", "ck05r3bor07h40d02y2hw4n4v", "token", "", ""}, "example_com": {"example.com", "test-org-id", "", "ck05r3bor07h40d02y2hw4n4v", "ck05r3bor07h40d02y2hw4n4v", "token", "", ""}}}, ctxs)
}

func (s *Suite) TestSetContextKey() {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContextKey("token", "test")
	outCtx, err := ctx.GetContext()
	s.NoError(err)
	s.Equal("test", outCtx.Token)
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
