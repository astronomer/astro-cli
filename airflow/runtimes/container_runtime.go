package runtimes

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/pkg/errors"
)

var spinnerCharSet = spinner.CharSets[14]

const (
	docker                         = "docker"
	podman                         = "podman"
	containerRuntimeNotFoundErrMsg = "Failed to find a container runtime. " +
		"See the Astro CLI prerequisites for more information. " +
		"https://www.astronomer.io/docs/astro/cli/install-cli"
	containerRuntimeInitMessage = " Astro uses container technology to run your Airflow project. " +
		"Please wait while we get things started…"
	spinnerRefresh = 100 * time.Millisecond
)

// ContainerRuntime interface defines the methods that manage
// the container runtime lifecycle.
type ContainerRuntime interface {
	Initialize() error
	Configure() error
	ConfigureOrKill() error
	Kill() error
}

// GetContainerRuntime creates a new container runtime based on the runtime string
// derived from the host machine configuration.
func GetContainerRuntime() (ContainerRuntime, error) {
	// Scan the environment for the container runtime binary.
	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return nil, err
	}

	// Return the appropriate container runtime based on the binary discovered.
	switch containerRuntime {
	case docker:
		return CreateDockerRuntime(new(DefaultDockerEngine)), nil
	case podman:
		return CreatePodmanRuntime(new(DefaultPodmanEngine)), nil
	default:
		return nil, errors.New(containerRuntimeNotFoundErrMsg)
	}
}

// FileChecker interface defines a method to check if a file exists.
// This is here mostly for testing purposes. This allows us to mock
// around actually checking for binaries on a live system as that
// would create inconsistencies across developer machines when
// working with the unit tests.
type FileChecker interface {
	Exists(path string) bool
}

// OSFileChecker is a concrete implementation of FileChecker.
type OSFileChecker struct{}

// Exists checks if the file exists in the file system.
func (f OSFileChecker) Exists(path string) bool {
	exists, _ := fileutil.Exists(path, nil)
	return exists
}

// FindBinary searches for the specified binary name in the provided $PATH directories,
// using the provided FileChecker. It searches each specific path within the systems
// $PATH environment variable for the binary concurrently and returns a boolean result
// indicating if the binary was found or not.
func FindBinary(pathEnv, binaryName string, checker FileChecker) bool {
	// Split the $PATH variable into it's individual paths,
	// using the OS specific path separator character.
	paths := strings.Split(pathEnv, string(os.PathListSeparator))

	// Although programs can be called without the .exe extension,
	// we need to append it here when searching the file system.
	if IsWindows() {
		binaryName += ".exe"
	}

	// Create a wait group to allow all binary search goroutines
	// to finish before we return from this function.
	var wg sync.WaitGroup
	found := make(chan string, 1)

	// Search each individual path concurrently.
	for _, dir := range paths {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			binaryPath := filepath.Join(dir, binaryName)
			if exists := checker.Exists(binaryPath); exists {
				select {
				// If the channel is open, send the path in, indicating a found binary.
				case found <- binaryPath:
				// If another goroutine has already sent a path into the channel
				// we'd be blocked. The default clause will run instead and effectively
				// skip sending the path into the channel, doing nothing, but allowing the
				// goroutine to complete without blocking.
				default:
				}
			}
		}(dir)
	}

	// Wait for the concurrent checks to finish and close the channel.
	wg.Wait()
	close(found)

	// If we found the binary in one of the paths, return true.
	if _, ok := <-found; ok {
		return true
	}

	// Otherwise the binary was not found, return false.
	return false
}

// GetContainerRuntimeBinary will return the manually configured container runtime,
// or search the $PATH for an acceptable runtime binary to use. This allows users
// to use alternative container runtimes without needing to explicitly configure it.
// Manual configuration should only be needed when both runtimes are installed and
// need to override to use one or the other and not use the auto-detection.
// We define this function as a variable so it can be mocked in unit tests to test
// higher-level functions.
var GetContainerRuntimeBinary = func() (string, error) {
	// Supported container runtime binaries
	binaries := []string{docker, podman}

	// If the binary is manually configured to an acceptable runtime, return it directly.
	// If a manual configuration exists, but it's not an appropriate runtime, we'll still
	// search the $PATH for an acceptable one before completely bailing out.
	configuredBinary := config.CFG.DockerCommand.GetString()
	if util.Contains(binaries, configuredBinary) {
		return configuredBinary, nil
	}

	// Get the $PATH environment variable.
	pathEnv := os.Getenv("PATH")
	for _, binary := range binaries {
		if found := FindBinary(pathEnv, binary, OSFileChecker{}); found {
			return binary, nil
		}
	}

	// If we made it here, no runtime was found, so we show a helpful error message
	// and halt the command execution.
	return "", errors.New("Failed to find a container runtime. " +
		"See the Astro CLI prerequisites for more information. " +
		"https://www.astronomer.io/docs/astro/cli/install-cli")
}
