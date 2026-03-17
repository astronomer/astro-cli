package proxy

import (
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const localhostSuffix = pkgproxy.LocalhostSuffix

// DeriveHostname converts a project directory path into a valid DNS hostname.
func DeriveHostname(projectDir string) (string, error) {
	return pkgproxy.DeriveHostname(projectDir)
}
