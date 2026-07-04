package container

import "runtime"

// OSChecker reports the host operating system. It is the public form of the
// package's internal host-OS checks, exported so callers (and the astro-cli
// adapter) can branch on platform without importing "runtime" directly. The
// astro-cli command hooks use this to, e.g., pre-create the Windows plugins
// directory.
type OSChecker interface {
	IsMac() bool
	IsWindows() bool
}

type osChecker struct{}

// CreateOSChecker returns the default OSChecker backed by runtime.GOOS.
func CreateOSChecker() OSChecker {
	return osChecker{}
}

func (osChecker) IsWindows() bool { return runtime.GOOS == "windows" }
func (osChecker) IsMac() bool     { return runtime.GOOS == "darwin" }

// IsPodman reports whether the given binary name is the podman binary. It is a
// small helper for call sites that hold the resolved binary string (from
// GetContainerRuntimeBinary) and need to branch on podman vs docker without
// re-resolving the engine.
func IsPodman(binaryName string) bool {
	return binaryName == podman
}

// GetContainerRuntimeBinary resolves the engine from cfg and returns the binary
// name to shell out to ("docker" or "podman"; OrbStack resolves to "docker").
// It mirrors astro-cli's original runtime-binary detection, now sharing the same
// resolution as Resolve.
func GetContainerRuntimeBinary(cfg Config) (string, error) {
	engine, err := Resolve(cfg)
	if err != nil {
		return "", err
	}
	return engine.Binary(), nil
}
