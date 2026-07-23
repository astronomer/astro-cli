package container

import (
	"errors"
	"fmt"
	"os"
)

const initMessage = "Astro uses containers to run your project. Please wait while we get started…"

// ErrMachineNotRunning is returned by ConnectionEnv when the engine is Podman
// but the astro-machine isn't up, so there's no socket to connect to. Callers
// typically surface this as "Airflow isn't running" rather than auto-starting.
var ErrMachineNotRunning = errors.New("the astro podman machine is not running")

// Manager resolves the host container engine once and exposes the engine-aware
// operations callers need: the connection environment to reach the running
// containers, and (for Podman) the astro-machine lifecycle. Construct it with
// NewManager.
type Manager struct {
	engine Engine
	cfg    Config
	fb     Feedback
	podman PodmanEngine
	host   host
}

// NewManager resolves the engine from cfg and returns a Manager. A nil Feedback
// is replaced with NoopFeedback.
func NewManager(cfg Config, fb Feedback) (*Manager, error) {
	engine, err := Resolve(cfg)
	if err != nil {
		return nil, err
	}
	if fb == nil {
		fb = NoopFeedback{}
	}
	return &Manager{engine: engine, cfg: cfg, fb: fb, podman: podmanEngine{}, host: defaultHost{}}, nil
}

// Engine returns the resolved engine.
func (m *Manager) Engine() Engine { return m.engine }

// ConnectionEnv returns the environment variables (KEY=VALUE) a child process
// must inherit so docker/podman commands target the right daemon. For Docker and
// OrbStack it returns nil (the default socket is correct). For Podman it inspects
// the running astro-machine and returns DOCKER_HOST (and CONTAINER_HOST off
// Windows). If DOCKER_HOST is already set in the environment, it returns nil so a
// user's own Podman workflow is not overridden. Returns ErrMachineNotRunning when
// the machine isn't up. This call has no side effects beyond the read-only
// `podman machine inspect`.
func (m *Manager) ConnectionEnv() ([]string, error) {
	if m.engine != Podman {
		return nil, nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return nil, nil
	}

	machine, err := m.podman.InspectMachine(podmanMachineName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMachineNotRunning, err)
	}
	if machine.State != podmanStatusRunning {
		return nil, ErrMachineNotRunning
	}

	return connectionEnvFor(machine, m.host.isWindows()), nil
}

// MachineState reports the astro-machine's lifecycle state. For non-Podman
// engines it always returns MachineAbsent (the concept doesn't apply).
func (m *Manager) MachineState() (MachineState, error) {
	if m.engine != Podman {
		return MachineAbsent, nil
	}
	machine, err := m.astroMachine()
	if err != nil {
		return "", err
	}
	switch {
	case machine == nil:
		return MachineAbsent, nil
	case machine.Running:
		return MachineRunning, nil
	default:
		return MachineStopped, nil
	}
}

// EnsureMachine brings the Podman astro-machine up (initializing it if absent,
// starting it if stopped) and sets it as the default connection. It is a no-op
// for Docker/OrbStack, and a no-op when the user has their own DOCKER_HOST set.
func (m *Manager) EnsureMachine() error {
	if m.engine != Podman {
		return nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return nil
	}

	m.fb.Start(initMessage)
	defer m.fb.Stop()

	// On Mac, only one machine may run at a time. If a non-astro machine is
	// running with no containers, stop it; otherwise leave the user's work alone.
	if other := m.otherRunningMachine(); other != "" && m.host.isMac() {
		if err := m.podman.SetMachineAsDefault(other); err != nil {
			return err
		}
		containers, err := m.podman.ListContainers()
		if err != nil {
			return err
		}
		if len(containers) > 0 {
			return errors.New(machineAlreadyRunningMsg)
		}
		if err := m.podman.StopMachine(other); err != nil {
			return err
		}
	}

	state, err := m.MachineState()
	if err != nil {
		return err
	}
	switch state {
	case MachineRunning:
		return m.podman.SetMachineAsDefault(podmanMachineName)
	case MachineStopped:
		if err := m.podman.StartMachine(podmanMachineName); err != nil {
			return err
		}
		return m.podman.SetMachineAsDefault(podmanMachineName)
	case MachineAbsent:
		if err := m.podman.InitializeMachine(podmanMachineName, m.cfg, m.fb); err != nil {
			return err
		}
		m.fb.Success("Astro machine initialized")
		return m.podman.SetMachineAsDefault(podmanMachineName)
	}
	return nil
}

// StartMachine starts the astro-machine (no-op for non-Podman engines).
func (m *Manager) StartMachine() error {
	if m.engine != Podman {
		return nil
	}
	return m.podman.StartMachine(podmanMachineName)
}

// StopMachine stops the astro-machine, but only when no other astro projects are
// still running on it. No-op for non-Podman engines.
func (m *Manager) StopMachine() error {
	if m.engine != Podman {
		return nil
	}
	machine, err := m.astroMachine()
	if err != nil {
		return err
	}
	if machine == nil || !machine.Running {
		return nil
	}
	containers, err := m.podman.ListContainers()
	if err != nil {
		return err
	}
	if m.hasRunningProjects(containers) {
		return nil
	}
	return m.podman.StopMachine(podmanMachineName)
}

// RemoveMachine stops (if needed) and removes the astro-machine. No-op for
// non-Podman engines or when the machine doesn't exist.
func (m *Manager) RemoveMachine() error {
	if m.engine != Podman {
		return nil
	}
	machine, err := m.astroMachine()
	if err != nil {
		return err
	}
	if machine == nil {
		return nil
	}
	// StopMachine is a no-op when the machine isn't running (and declines to stop
	// when other projects are still using it).
	if err := m.StopMachine(); err != nil {
		return err
	}
	return m.podman.RemoveMachine(podmanMachineName)
}

// astroMachine returns the listed astro-machine, or nil if it doesn't exist.
func (m *Manager) astroMachine() (*ListedMachine, error) {
	machines, err := m.podman.ListMachines()
	if err != nil {
		return nil, err
	}
	for i := range machines {
		if machines[i].Name == podmanMachineName {
			return &machines[i], nil
		}
	}
	return nil, nil
}

// otherRunningMachine returns the name of a running, non-astro machine, if any.
func (m *Manager) otherRunningMachine() string {
	machines, _ := m.podman.ListMachines()
	for _, machine := range machines {
		if machine.Running && machine.Name != podmanMachineName {
			return machine.Name
		}
	}
	return ""
}

// hasRunningProjects reports whether any compose projects are running among the
// given containers (by the compose project label).
func (m *Manager) hasRunningProjects(containers []ListedContainer) bool {
	for _, c := range containers {
		if _, ok := c.Labels[composeProjectLabel]; ok {
			return true
		}
	}
	return false
}
