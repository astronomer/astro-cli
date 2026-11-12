package config

import (
	"bytes"
	"strings"

	"github.com/spf13/afero"
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

func (s *Suite) TestContextGetPublicRESTAPIURL() {
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
		s.Run(tt.name, func() {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetPublicRESTAPIURL("v1alpha1"); got != tt.want {
				s.Fail("Context.GetPublicRESTAPIURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestPrintCurrentCloudContext() {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContext()
	ctx.SwitchContext()
	buf := new(bytes.Buffer)
	err := PrintCurrentCloudContext(buf)
	s.NoError(err)
	s.Contains(buf.String(), "localhost")
}

// cloud.api.protocol is a scheme and nothing more: a value carrying a host
// falls back to https rather than sending the API calls, and the token on
// them, somewhere else.
func (s *Suite) TestContextAPIProtocolIsOnlyAScheme() {
	initTestConfig()
	defer CFG.CloudAPIProtocol.SetHomeString("https") //nolint:errcheck
	c := Context{Domain: "astronomer.io"}
	for value, want := range map[string]string{
		"https":                   "https://api.astronomer.io/v1alpha1",
		"http":                    "http://api.astronomer.io/v1alpha1",
		"HTTP\t":                  "http://api.astronomer.io/v1alpha1",
		"https://evil.example/x?": "https://api.astronomer.io/v1alpha1",
		"":                        "https://api.astronomer.io/v1alpha1",
	} {
		s.NoError(CFG.CloudAPIProtocol.SetHomeString(value))
		s.Equal(want, c.GetPublicRESTAPIURL("v1alpha1"), value)
		scheme, _, _ := strings.Cut(want, "://")
		s.True(strings.HasPrefix(c.GetSoftwareAPIURL(), scheme+"://houston.astronomer.io:"), value)
	}
}
