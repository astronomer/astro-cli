package container

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePodman is a PodmanEngine stub for tests.
type fakePodman struct {
	inspect    *InspectedMachine
	inspectErr error
	machines   []ListedMachine
	containers []ListedContainer
}

func (f fakePodman) InitializeMachine(string, Config, Feedback) error { return nil }
func (f fakePodman) StartMachine(string) error                       { return nil }
func (f fakePodman) StopMachine(string) error                        { return nil }
func (f fakePodman) RemoveMachine(string) error                      { return nil }
func (f fakePodman) SetMachineAsDefault(string) error                { return nil }
func (f fakePodman) InspectMachine(string) (*InspectedMachine, error) {
	return f.inspect, f.inspectErr
}
func (f fakePodman) ListMachines() ([]ListedMachine, error)     { return f.machines, nil }
func (f fakePodman) ListContainers() ([]ListedContainer, error) { return f.containers, nil }

func runningMachine() *InspectedMachine {
	return &InspectedMachine{
		Name:           "astro-machine",
		State:          podmanStatusRunning,
		ConnectionInfo: ConnectionInfo{PodmanSocket: PodmanSocket{Path: "/tmp/podman.sock"}},
	}
}

func TestConnectionEnv(t *testing.T) {
	t.Run("docker returns nil", func(t *testing.T) {
		m := &Manager{engine: Docker, podman: fakePodman{}, host: fakeHost{}, fb: NoopFeedback{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})

	t.Run("podman running returns DOCKER_HOST and CONTAINER_HOST", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "") // ensure not treated as pre-set
		m := &Manager{engine: Podman, podman: fakePodman{inspect: runningMachine()}, host: fakeHost{}, fb: NoopFeedback{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, []string{
			"DOCKER_HOST=unix:///tmp/podman.sock",
			"CONTAINER_HOST=unix:///tmp/podman.sock",
		}, env)
	})

	t.Run("podman on windows omits CONTAINER_HOST and uses npipe", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: fakePodman{inspect: runningMachine()}, host: fakeHost{windows: true}, fb: NoopFeedback{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, []string{"DOCKER_HOST=npipe:////./pipe/podman-astro-machine"}, env)
	})

	t.Run("podman stopped returns ErrMachineNotRunning", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		stopped := runningMachine()
		stopped.State = podmanStatusStopped
		m := &Manager{engine: Podman, podman: fakePodman{inspect: stopped}, host: fakeHost{}, fb: NoopFeedback{}}
		_, err := m.ConnectionEnv()
		assert.ErrorIs(t, err, ErrMachineNotRunning)
	})

	t.Run("podman inspect error returns ErrMachineNotRunning", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: fakePodman{inspectErr: errors.New("not found")}, host: fakeHost{}, fb: NoopFeedback{}}
		_, err := m.ConnectionEnv()
		assert.ErrorIs(t, err, ErrMachineNotRunning)
	})

	t.Run("pre-set DOCKER_HOST is respected", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "tcp://example:1234")
		m := &Manager{engine: Podman, podman: fakePodman{inspect: runningMachine()}, host: fakeHost{}, fb: NoopFeedback{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})
}

func TestMachineState(t *testing.T) {
	t.Run("docker is absent", func(t *testing.T) {
		m := &Manager{engine: Docker, podman: fakePodman{}, host: fakeHost{}, fb: NoopFeedback{}}
		st, err := m.MachineState()
		require.NoError(t, err)
		assert.Equal(t, MachineAbsent, st)
	})

	t.Run("podman running", func(t *testing.T) {
		m := &Manager{engine: Podman, podman: fakePodman{machines: []ListedMachine{{Name: "astro-machine", Running: true}}}, host: fakeHost{}, fb: NoopFeedback{}}
		st, err := m.MachineState()
		require.NoError(t, err)
		assert.Equal(t, MachineRunning, st)
	})

	t.Run("podman stopped", func(t *testing.T) {
		m := &Manager{engine: Podman, podman: fakePodman{machines: []ListedMachine{{Name: "astro-machine", Running: false}}}, host: fakeHost{}, fb: NoopFeedback{}}
		st, err := m.MachineState()
		require.NoError(t, err)
		assert.Equal(t, MachineStopped, st)
	})

	t.Run("podman absent", func(t *testing.T) {
		m := &Manager{engine: Podman, podman: fakePodman{machines: []ListedMachine{{Name: "other", Running: true}}}, host: fakeHost{}, fb: NoopFeedback{}}
		st, err := m.MachineState()
		require.NoError(t, err)
		assert.Equal(t, MachineAbsent, st)
	})
}
