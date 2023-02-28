package config

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var err error

func TestNewTableOut(t *testing.T) {
	tab := newTableOut()
	assert.NotNil(t, tab)
	assert.Equal(t, []int{36, 36}, tab.Padding)
}

func TestGetCurrentContextError(t *testing.T) {
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
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
}

func TestPrintContext(t *testing.T) {
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
	assert.NoError(t, err)
	expected := " CONTROLPLANE                        WORKSPACE                           \n example.com                         ck05r3bor07h40d02y2hw4n4v           \n"
	assert.Equal(t, expected, buf.String())
}

func TestPrintContextNA(t *testing.T) {
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
	assert.NoError(t, err)
	expected := " CONTROLPLANE                        WORKSPACE                           \n example.com                         N/A                                 \n"
	assert.Equal(t, expected, buf.String())
}

func TestGetCurrentContext(t *testing.T) {
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
	assert.NoError(t, err)
	assert.Equal(t, "example.com", ctx.Domain)
	assert.Equal(t, "token", ctx.Token)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", ctx.Workspace)
}

func TestDeleteContext(t *testing.T) {
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
	assert.NoError(t, err)

	ctx = Context{}
	err = ctx.DeleteContext()
	assert.ErrorIs(t, err, ErrCtxConfigErr)
}

func TestResetCurrentContext(t *testing.T) {
	initTestConfig()
	err := ResetCurrentContext()
	assert.NoError(t, err)
	ctx, err := GetCurrentContext()
	assert.Equal(t, "", ctx.Domain)
	assert.ErrorIs(t, err, ErrGetHomeString)
}

func TestGetContexts(t *testing.T) {
	initTestConfig()
	ctxs, err := GetContexts()
	assert.NoError(t, err)
	assert.Equal(t, Contexts{Contexts: map[string]Context{"test_com": {"test.com", "test-org-id", "test-org-short-name", "ck05r3bor07h40d02y2hw4n4v", "ck05r3bor07h40d02y2hw4n4v", "token", "", ""}, "example_com": {"example.com", "test-org-id", "test-org-short-name", "ck05r3bor07h40d02y2hw4n4v", "ck05r3bor07h40d02y2hw4n4v", "token", "", ""}}}, ctxs)
}

func TestSetContextKey(t *testing.T) {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContextKey("token", "test")
	outCtx, err := ctx.GetContext()
	assert.NoError(t, err)
	assert.Equal(t, "test", outCtx.Token)
}

func TestSetOrganizationContext(t *testing.T) {
	initTestConfig()
	t.Run("set organization context", func(t *testing.T) {
		ctx := Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "org_short_name_1")
		outCtx, err := ctx.GetContext()
		assert.NoError(t, err)
		assert.Equal(t, "org1", outCtx.Organization)
		assert.Equal(t, "org_short_name_1", outCtx.OrganizationShortName)
	})

	t.Run("set organization context error", func(t *testing.T) {
		ctx := Context{Domain: ""}
		assert.NoError(t, err)
		err = ctx.SetOrganizationContext("org1", "org_short_name_1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context config invalid, no domain specified")
	})
}

func TestExpiresIn(t *testing.T) {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	err := ctx.SetExpiresIn(12)
	assert.NoError(t, err)

	outCtx, err := ctx.GetContext()
	assert.NoError(t, err)

	val, err := outCtx.GetExpiresIn()
	assert.NoError(t, err)
	assert.Equal(t, "localhost", outCtx.Domain)
	assert.True(t, time.Now().Add(time.Duration(12)*time.Second).After(val)) // now + 12 seconds will always be after expire time, since that is set before
}

func TestExpiresInFailure(t *testing.T) {
	initTestConfig()
	ctx := Context{}
	err := ctx.SetExpiresIn(1)
	assert.ErrorIs(t, err, ErrCtxConfigErr)

	val, err := ctx.GetExpiresIn()
	assert.ErrorIs(t, err, ErrCtxConfigErr)
	assert.Equal(t, time.Time{}, val)
}
