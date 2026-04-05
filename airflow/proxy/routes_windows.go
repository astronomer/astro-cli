//go:build windows

package proxy

import (
	"os"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

func acquireLock() (*os.File, error) {
	ensureInit()
	return pkgproxy.AcquireLock()
}

func releaseLock(f *os.File) {
	pkgproxy.ReleaseLock(f)
}

var isPIDAlive = pkgproxy.IsPIDAlive
