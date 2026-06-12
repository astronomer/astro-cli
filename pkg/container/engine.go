package container

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

const (
	docker   = "docker"
	podman   = "podman"
	orbstack = "orbstack"
	orbctl   = "orbctl"

	notFoundMsg = "Failed to find a container runtime. " +
		"See the Astro CLI prerequisites for more information. " +
		"https://www.astronomer.io/docs/astro/cli/install-cli"
)

// Engine is the resolved container runtime.
type Engine string

const (
	Docker   Engine = "docker"
	Podman   Engine = "podman"
	Orbstack Engine = "orbstack"
)

// Binary returns the executable to shell out to. Docker and OrbStack are both
// driven through the "docker" binary; only Podman uses "podman".
func (e Engine) Binary() string {
	if e == Podman {
		return podman
	}
	return docker
}

// IsPodman reports whether the engine is Podman.
func (e Engine) IsPodman() bool { return e == Podman }

// host abstracts host queries so Resolve is unit-testable without touching the
// real filesystem or $PATH.
type host interface {
	isWindows() bool
	isMac() bool
	fileExists(path string) bool
	pathEnv() string
}

type defaultHost struct{}

func (defaultHost) isWindows() bool          { return runtime.GOOS == "windows" }
func (defaultHost) isMac() bool              { return runtime.GOOS == "darwin" }
func (defaultHost) fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
func (defaultHost) pathEnv() string          { return os.Getenv("PATH") }

// Resolve picks the engine: honor cfg.Binary when it names a supported runtime,
// otherwise search $PATH (docker first, then podman). When docker is selected
// and the orbctl binary is present, the engine is OrbStack (still driven through
// the docker binary). Pure with respect to global state so callers agree with
// astro-cli's own resolution.
func Resolve(cfg Config) (Engine, error) { return resolveWith(cfg, defaultHost{}) }

func resolveWith(cfg Config, h host) (Engine, error) {
	bin, err := resolveBinary(cfg, h)
	if err != nil {
		return "", err
	}
	if bin == podman {
		return Podman, nil
	}
	// docker — orbctl on $PATH signals OrbStack.
	if findBinary(h.pathEnv(), orbctl, h) {
		return Orbstack, nil
	}
	return Docker, nil
}

func resolveBinary(cfg Config, h host) (string, error) {
	binaries := []string{docker, podman}
	if slices.Contains(binaries, cfg.Binary) {
		return cfg.Binary, nil
	}
	pathEnv := h.pathEnv()
	for _, b := range binaries {
		if findBinary(pathEnv, b, h) {
			return b, nil
		}
	}
	return "", errors.New(notFoundMsg)
}

// findBinary concurrently searches the $PATH directories for binaryName.
func findBinary(pathEnv, binaryName string, h host) bool {
	if h.isWindows() {
		binaryName += ".exe"
	}
	paths := strings.Split(pathEnv, string(os.PathListSeparator))

	var wg sync.WaitGroup
	found := make(chan struct{}, 1)
	for _, dir := range paths {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			if h.fileExists(filepath.Join(dir, binaryName)) {
				select {
				case found <- struct{}{}:
				default:
				}
			}
		}(dir)
	}
	wg.Wait()
	close(found)
	_, ok := <-found
	return ok
}
