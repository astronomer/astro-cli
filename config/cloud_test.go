package config

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func initTestConfig() {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: test_com
contexts:
  example_com:
    domain: example.com
    organization: test-org-id
    organization_short_name: test-org-short-name
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
  test_com:
    domain: test.com
    organization: test-org-id
    organization_short_name: test-org-short-name
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	_ = afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	InitConfig(fs)
}

func TestContextGetPublicGraphQLAPIURL(t *testing.T) {
	initTestConfig()
	CFG.CloudAPIProtocol.SetHomeString("https")
	type fields struct {
		Domain string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "basic localhost case",
			fields: fields{Domain: "localhost"},
			want:   "http://localhost:8871/graphql",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "cloud.astro.io"},
			want:   "https://api.astro.io/hub/graphql",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetPublicGraphQLAPIURL(); got != tt.want {
				t.Errorf("Context.GetPublicGraphQLAPIURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextGetPublicRESTAPIURL(t *testing.T) {
	initTestConfig()
	CFG.CloudAPIProtocol.SetHomeString("https")
	type fields struct {
		Domain string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "basic localhost case",
			fields: fields{Domain: "localhost"},
			want:   "http://localhost:8888/v1alpha1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "cloud.astro.io"},
			want:   "https://api.astro.io/v1alpha1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetPublicRESTAPIURL("v1alpha1"); got != tt.want {
				t.Errorf("Context.GetPublicRESTAPIURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrintCurrentCloudContext(t *testing.T) {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContext()
	ctx.SwitchContext()
	buf := new(bytes.Buffer)
	err := PrintCurrentCloudContext(buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "localhost")
}
