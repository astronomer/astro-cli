package container

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMachineNotRunning is returned by ConnectionEnv when the engine is Podman on
// a host that needs a machine (Mac, Windows) and none is running, so there's no
// socket to connect to. Astro does not create or start podman machines; the
// machine is the user's to run, so each return wraps this with the command
// that fixes the case at hand.
var ErrMachineNotRunning = errors.New("no podman machine is running")

// Manager resolves the host container engine once and exposes the connection
// environment that reaches it — for Podman, the machine the user is running.
// Construct it with NewManager.
type Manager struct {
	engine Engine
	podman PodmanEngine
	host   host
}

// NewManager resolves the engine from cfg and returns a Manager. The Feedback
// goes unused, since nothing the Manager does is long-running; it stays in the
// signature so callers (Astro Desktop among them) keep compiling.
func NewManager(cfg Config, _ Feedback) (*Manager, error) {
	engine, err := Resolve(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{engine: engine, podman: podmanEngine{}, host: defaultHost{}}, nil
}

// Engine returns the resolved engine.
func (m *Manager) Engine() Engine { return m.engine }

// Initialize implements ContainerRuntime for Podman. It starts nothing: it
// checks that the engine is reachable, returning ErrMachineNotRunning when the
// host needs a machine and none is up.
func (m *Manager) Initialize() error {
	_, err := m.ConnectionEnv()
	return err
}

// ConnectionEnv returns the environment variables (KEY=VALUE) a child process
// must inherit so docker/podman commands target the right daemon. For Docker and
// OrbStack it returns nil (the default socket is correct). For Podman it finds
// the running machine (the default one, when several are up) and returns
// DOCKER_HOST (and CONTAINER_HOST off Windows) pointed at its socket. Podman on
// Linux runs natively, so there no running machine means nil; on Mac and Windows
// it means ErrMachineNotRunning. If DOCKER_HOST is already set in the
// environment, it returns nil so a user's own Podman workflow is not overridden.
// This call has no side effects beyond the read-only `podman machine ls` and
// `podman machine inspect`.
func (m *Manager) ConnectionEnv() ([]string, error) {
	if m.engine != Podman {
		return nil, nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return nil, nil
	}

	needsMachine := m.host.isMac() || m.host.isWindows()
	machines, err := m.podman.ListMachines()
	if err != nil {
		if !needsMachine {
			return nil, nil
		}
		// Not ErrMachineNotRunning: podman did not answer, which is a podman
		// that is missing or broken, not a machine that is down. With
		// container.binary pinning podman on a host without it, this is
		// "podman not found", and callers report it as no engine.
		return nil, err
	}
	windows := m.host.isWindows()
	name := runningMachine(machines, windows)
	if name == "" {
		if windows && anyRunning(machines) {
			const makeDefault = "make the running one the default with podman system connection default <name>"
			if stopped := stoppedMachine(machines); stopped != "" {
				return nil, fmt.Errorf("%w as the default; start it with podman machine start %s, or "+makeDefault,
					ErrMachineNotRunning, stopped)
			}
			return nil, fmt.Errorf("%w as the default; "+makeDefault, ErrMachineNotRunning)
		}
		if !needsMachine {
			return nil, nil
		}
		if len(machines) == 0 {
			// `podman machine start` has nothing to start here. This is the
			// state most people upgrading from astro's own machine are in: it
			// made astro-machine on start and removed it on stop.
			return nil, fmt.Errorf("%w, and none exists yet; create and start one with podman machine init --now", ErrMachineNotRunning)
		}
		// Named: a bare `podman machine start` starts only
		// podman-machine-default, which fails for a leftover astro-machine or
		// any machine the user named themselves.
		return nil, fmt.Errorf("%w; start it with podman machine start %s", ErrMachineNotRunning, stoppedMachine(machines))
	}

	// A machine ls just listed as running that inspect cannot read is a podman
	// fault, not a machine that is down, so it is passed through as one.
	machine, err := m.podman.InspectMachine(name)
	if err != nil {
		return nil, err
	}
	return connectionEnvFor(machine, windows), nil
}

// runningMachine picks the machine to connect to: the default one when it is
// running, otherwise (off Windows) the first running machine, or "" when none
// is up. Windows gets no fallback: CONTAINER_HOST is not set there, so native
// podman commands follow the default connection, and pointing DOCKER_HOST at
// any other machine would split compose and podman across two machines.
func runningMachine(machines []ListedMachine, windows bool) string {
	name := ""
	for _, machine := range machines {
		if !machine.Running {
			continue
		}
		if machine.Default {
			return machine.Name
		}
		if name == "" && !windows {
			name = machine.Name
		}
	}
	return name
}

// stoppedMachine names the machine a start should bring up: the default one
// when it is stopped, otherwise the first stopped machine. Called only when
// some machine is stopped.
func stoppedMachine(machines []ListedMachine) string {
	name := ""
	for _, machine := range machines {
		if machine.Running {
			continue
		}
		if machine.Default {
			return machine.Name
		}
		if name == "" {
			name = machine.Name
		}
	}
	return name
}

// anyRunning reports whether any listed machine is running.
func anyRunning(machines []ListedMachine) bool {
	for _, machine := range machines {
		if machine.Running {
			return true
		}
	}
	return false
}

// connectionEnvFor computes the DOCKER_HOST (and, off Windows, CONTAINER_HOST)
// environment for a running machine.
func connectionEnvFor(machine *InspectedMachine, windows bool) []string {
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	if windows {
		dockerHost = "npipe:////./pipe/" + pipeName(machine)
	}
	env := []string{"DOCKER_HOST=" + dockerHost}
	// CONTAINER_HOST routes native podman commands (e.g. `podman build`) to the
	// machine. The npipe:// scheme isn't supported by native podman on Windows,
	// where the default-connection setting handles routing instead.
	if !windows {
		env = append(env, "CONTAINER_HOST="+dockerHost)
	}
	return env
}

// pipeName is the machine's Windows named pipe: the one `podman machine
// inspect` reports, or, from a podman too old to report it, podman's own rule —
// a "podman-" prefix only on names that do not already start with "podman", so
// the default machine listens on podman-machine-default, not
// podman-podman-machine-default.
func pipeName(machine *InspectedMachine) string {
	if p := machine.ConnectionInfo.PodmanPipe.Path; p != "" {
		return p[strings.LastIndexAny(p, `\/`)+1:]
	}
	if strings.HasPrefix(machine.Name, "podman") {
		return machine.Name
	}
	return "podman-" + machine.Name
}
