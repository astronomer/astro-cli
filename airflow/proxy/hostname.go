package proxy

import (
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const localhostSuffix = pkgproxy.LocalhostSuffix

// DeriveHostname converts a project name and directory into a valid DNS hostname.
// projectName (from .astro/config.yaml's project.name) is preferred over the
// directory name when set. See pkg/proxy.DeriveHostname for full semantics.
func DeriveHostname(projectName, projectDir string) (string, error) {
	return pkgproxy.DeriveHostname(projectName, projectDir)
}
