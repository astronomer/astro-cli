package config

import (
	"strconv"
	"strings"
)

// The settings below are spliced into the authority of every Astro and
// Houston API URL, in front of or after the host. Each is read as the one
// thing it names, a scheme or a port, and falls back to its default on
// anything else: a value carrying a host (https://other.example/x? as the
// protocol, 443@other.example as the port) would otherwise move every request,
// and the token on it, to another host.

const (
	defaultAPIProtocol = "https"
	defaultWSProtocol  = "wss"
	defaultAPIPort     = "443"
)

// apiProtocol is internal.api.protocol when it says http, https otherwise.
func apiProtocol() string {
	if strings.EqualFold(strings.TrimSpace(CFG.CloudAPIProtocol.GetString()), "http") {
		return "http"
	}
	return defaultAPIProtocol
}

// wsProtocol is internal.apc.api.ws_protocol when it says ws, wss otherwise.
func wsProtocol() string {
	if strings.EqualFold(strings.TrimSpace(CFG.APCWSProtocol.GetString()), "ws") {
		return "ws"
	}
	return defaultWSProtocol
}

// apiPort is internal.apc.api.port when it is a port number, 443 otherwise.
func apiPort() string {
	if n, err := strconv.Atoi(strings.TrimSpace(CFG.APCAPIPort.GetString())); err == nil && n >= 1 && n <= 65535 {
		return strconv.Itoa(n)
	}
	return defaultAPIPort
}
