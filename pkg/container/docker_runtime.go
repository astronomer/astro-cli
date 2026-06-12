package container

import (
	"errors"
	"time"
)

const (
	defaultTimeoutSeconds = 60
	tickNum               = 500
	open                  = "open"
	timeoutErrMsg         = "timed out waiting for docker"
	dockerOpenNotice      = "We couldn't start the docker engine automatically. Please start it manually and try again."
)

// DockerEngine abstracts the two Docker operations the runtime needs so tests
// can substitute a fake without a live Docker install. The concrete
// implementations are dockerEngine and orbstackEngine.
type DockerEngine interface {
	IsRunning() (string, error)
	Start() (string, error)
}

// dockerEngine drives Docker Desktop through the `docker`/`open` binaries.
type dockerEngine struct{}

func (dockerEngine) IsRunning() (string, error) {
	return (&command{binary: docker, args: []string{"ps"}}).execute()
}

func (dockerEngine) Start() (string, error) {
	return (&command{binary: open, args: []string{"-a", docker}}).execute()
}

// orbstackEngine drives OrbStack: still checked through the `docker` binary, but
// started via `open -a orbstack`.
type orbstackEngine struct {
	dockerEngine
}

func (orbstackEngine) Start() (string, error) {
	return (&command{binary: open, args: []string{"-a", orbstack}}).execute()
}

// DockerRuntime is the ContainerRuntime for Docker and OrbStack. Initialize
// auto-starts the engine on Mac; Configure/ConfigureOrKill/Kill are no-ops
// because the default Docker socket needs no per-process setup.
type DockerRuntime struct {
	Engine    DockerEngine
	OSChecker OSChecker
	fb        Feedback
}

// newDockerRuntime builds a DockerRuntime for the resolved engine, selecting the
// orbstack engine for OrbStack and the docker engine otherwise.
func newDockerRuntime(engine Engine, fb Feedback) *DockerRuntime {
	var de DockerEngine = dockerEngine{}
	if engine == Orbstack {
		de = orbstackEngine{}
	}
	return &DockerRuntime{Engine: de, OSChecker: CreateOSChecker(), fb: fb}
}

// CreateDockerRuntime builds a DockerRuntime from an explicit engine and OS
// checker. It uses NoopFeedback; provided primarily for tests.
func CreateDockerRuntime(engine DockerEngine, osChecker OSChecker) *DockerRuntime {
	return &DockerRuntime{Engine: engine, OSChecker: osChecker, fb: NoopFeedback{}}
}

// Initialize starts Docker if it isn't already running. We only attempt this on
// Mac today; elsewhere it's a no-op.
func (rt *DockerRuntime) Initialize() error {
	if !rt.OSChecker.IsMac() {
		return nil
	}
	return rt.initializeDocker(defaultTimeoutSeconds)
}

func (rt *DockerRuntime) Configure() error       { return nil }
func (rt *DockerRuntime) ConfigureOrKill() error { return nil }
func (rt *DockerRuntime) Kill() error            { return nil }

// initializeDocker checks whether Docker is running (`docker ps`) and, if not,
// attempts to start it (`open -a docker`) and polls until it comes up or the
// timeout elapses.
func (rt *DockerRuntime) initializeDocker(timeoutSeconds int) error {
	fb := rt.fb
	if fb == nil {
		fb = NoopFeedback{}
	}

	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Millisecond)
	defer ticker.Stop()

	// `docker ps` succeeding means Docker is already running.
	if _, err := rt.Engine.IsRunning(); err == nil {
		return nil
	}

	// Docker isn't running; try to start it.
	if _, err := rt.Engine.Start(); err != nil {
		return errors.New(dockerOpenNotice)
	}

	fb.Start(initMessage)
	defer fb.Stop()
	for {
		select {
		case <-timeout:
			return errors.New(timeoutErrMsg)
		case <-ticker.C:
			if _, err := rt.Engine.IsRunning(); err != nil {
				continue
			}
			return nil
		}
	}
}
