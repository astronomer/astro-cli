package config

import (
	"bytes"
)

func (s *Suite) TestContextGetSoftwareAPIURL() {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
	CFG.CloudAPIProtocol.SetHomeString("https")
	CFG.CloudAPIPort.SetHomeString("8080")
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
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "https://houston.dev.astro.io:8080/v1",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareAPIURL(); got != tt.want {
				s.Fail("Context.GetSoftwareAPIURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestContextGetSoftwareAppURL() {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
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
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "https://app.dev.astro.io",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareAppURL(); got != tt.want {
				s.Fail("Context.GetSoftwareAppURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestContextGetSoftwareWebsocketURL() {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
	CFG.CloudAPIPort.SetHomeString("8080")
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
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "wss://houston.dev.astro.io:8080/ws",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareWebsocketURL(); got != tt.want {
				s.Fail("Context.GetSoftwareWebsocketURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestPrintCurrentSoftwareContext() {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContext()
	ctx.SwitchContext()
	buf := new(bytes.Buffer)
	err := PrintCurrentSoftwareContext(buf)
	s.NoError(err)
	s.Contains(buf.String(), "localhost")
}
