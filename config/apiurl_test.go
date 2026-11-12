package config

// The API URLs keep their host whatever the protocol and port settings say:
// a value carrying a host falls back to the default rather than sending the
// calls, and the token on them, somewhere else.
func (s *Suite) TestAPIURLsKeepTheirHost() {
	initTestConfig()
	defer CFG.CloudAPIProtocol.SetHomeString(defaultAPIProtocol)
	defer CFG.APCWSProtocol.SetHomeString(defaultWSProtocol)
	defer CFG.APCAPIPort.SetHomeString(defaultAPIPort)

	astro := Context{Domain: "astronomer.io"}
	apc := Context{Domain: "astro.example.com"}
	for _, tc := range []struct {
		protocol, ws, port      string
		rest, houston, app, wss string
	}{
		{
			"https", "wss", "8080",
			"https://api.astronomer.io/v1", "https://houston.astro.example.com:8080/v1", "https://app.astro.example.com", "wss://houston.astro.example.com:8080/ws",
		},
		{
			"HTTP\t", "WS", "443",
			"http://api.astronomer.io/v1", "http://houston.astro.example.com:443/v1", "http://app.astro.example.com", "ws://houston.astro.example.com:443/ws",
		},
		{
			"https://evil.example/x?", "wss://evil.example/x?", "443@evil.example",
			"https://api.astronomer.io/v1", "https://houston.astro.example.com:443/v1", "https://app.astro.example.com", "wss://houston.astro.example.com:443/ws",
		},
		{
			"", "", "70000",
			"https://api.astronomer.io/v1", "https://houston.astro.example.com:443/v1", "https://app.astro.example.com", "wss://houston.astro.example.com:443/ws",
		},
	} {
		s.NoError(CFG.CloudAPIProtocol.SetHomeString(tc.protocol))
		s.NoError(CFG.APCWSProtocol.SetHomeString(tc.ws))
		s.NoError(CFG.APCAPIPort.SetHomeString(tc.port))
		s.Equal(tc.rest, astro.GetPublicRESTAPIURL("v1"), tc.protocol)
		s.Equal(tc.houston, apc.GetSoftwareAPIURL(), tc.protocol+" "+tc.port)
		s.Equal(tc.app, apc.GetSoftwareAppURL(), tc.protocol)
		s.Equal(tc.wss, apc.GetSoftwareWebsocketURL(), tc.ws+" "+tc.port)
	}
}
