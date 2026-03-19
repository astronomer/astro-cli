//go:build !windows

package proxy

import pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"

var isPIDAlive = pkgproxy.IsPIDAlive
