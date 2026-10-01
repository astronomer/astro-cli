package container

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePodman is a PodmanEngine stub for tests. inspected records the name
// InspectMachine was asked for, so a case can check which machine was picked.
type fakePodman struct {
	machines   []ListedMachine
	listErr    error
	inspectErr error
	inspected  *string
	pipe       string // the PodmanPipe path inspect reports; "" for an older podman
}

func (f *fakePodman) InspectMachine(name string) (*InspectedMachine, error) {
	if f.inspected != nil {
		*f.inspected = name
	}
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	return &InspectedMachine{
		Name:  name,
		State: "running",
		ConnectionInfo: ConnectionInfo{
			PodmanSocket: PodmanSocket{Path: "/tmp/" + name + ".sock"},
			PodmanPipe:   PodmanPipe{Path: f.pipe},
		},
	}, nil
}
func (f *fakePodman) ListMachines() ([]ListedMachine, error) { return f.machines, f.listErr }

func running(name string) ListedMachine { return ListedMachine{Name: name, Running: true} }

func TestConnectionEnv(t *testing.T) {
	mac := fakeHost{mac: true}

	t.Run("docker returns nil", func(t *testing.T) {
		m := &Manager{engine: Docker, podman: &fakePodman{}, host: mac}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})

	t.Run("podman running returns DOCKER_HOST and CONTAINER_HOST", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "") // ensure not treated as pre-set
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{running("podman-machine-default")}}, host: mac}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, []string{
			"DOCKER_HOST=unix:///tmp/podman-machine-default.sock",
			"CONTAINER_HOST=unix:///tmp/podman-machine-default.sock",
		}, env)
	})

	t.Run("podman reaches a machine of any name", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		var inspected string
		m := &Manager{engine: Podman, podman: &fakePodman{
			machines:  []ListedMachine{{Name: "stopped"}, running("mine")},
			inspected: &inspected,
		}, host: mac}
		_, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, "mine", inspected)
	})

	t.Run("podman prefers the default machine when several run", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		var inspected string
		def := running("default-one")
		def.Default = true
		m := &Manager{engine: Podman, podman: &fakePodman{
			machines:  []ListedMachine{running("first"), def},
			inspected: &inspected,
		}, host: fakeHost{}}
		_, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, "default-one", inspected)
	})

	windowsDefault := func(name string) []ListedMachine {
		lm := running(name)
		lm.Default = true
		return []ListedMachine{lm}
	}

	t.Run("podman on windows omits CONTAINER_HOST and uses the reported pipe", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{
			machines: windowsDefault("podman-machine-default"),
			pipe:     `\\.\pipe\podman-machine-default`,
		}, host: fakeHost{windows: true}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Equal(t, []string{"DOCKER_HOST=npipe:////./pipe/podman-machine-default"}, env)
	})

	// Without a reported pipe, podman's own naming rule applies: the prefix is
	// added only to names that lack it, never doubled.
	for name, want := range map[string]string{
		"podman-machine-default": "podman-machine-default",
		"dev":                    "podman-dev",
	} {
		t.Run("podman on windows derives the pipe for "+name, func(t *testing.T) {
			t.Setenv("DOCKER_HOST", "")
			m := &Manager{engine: Podman, podman: &fakePodman{machines: windowsDefault(name)}, host: fakeHost{windows: true}}
			env, err := m.ConnectionEnv()
			require.NoError(t, err)
			assert.Equal(t, []string{"DOCKER_HOST=npipe:////./pipe/" + want}, env)
		})
	}

	t.Run("podman on windows refuses a running machine that is not the default", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{
			machines: []ListedMachine{{Name: "podman-machine-default", Default: true}, running("dev")},
		}, host: fakeHost{windows: true}}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.Contains(t, err.Error(), "podman machine start podman-machine-default")
		assert.Contains(t, err.Error(), "podman system connection default")
	})

	t.Run("podman on windows with every machine running but none default names no start", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{running("dev")}}, host: fakeHost{windows: true}}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.NotContains(t, err.Error(), "podman machine start")
		assert.Contains(t, err.Error(), "podman system connection default")
	})

	// Named, because a bare `podman machine start` starts only
	// podman-machine-default and fails for any other name — a leftover
	// astro-machine above all.
	t.Run("podman with a stopped machine on mac names it in the start command", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{{Name: "astro-machine"}}}, host: mac}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.Contains(t, err.Error(), "`podman machine start astro-machine`")
	})

	t.Run("podman names the default among several stopped machines", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{{Name: "first"}, {Name: "mine", Default: true}}}, host: mac}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.Contains(t, err.Error(), "`podman machine start mine`")
	})

	t.Run("podman with no machine at all on mac says to create one", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{}, host: mac}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.Contains(t, err.Error(), "podman machine init --now")
	})

	t.Run("podman with no running machine on linux runs natively", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{}, host: fakeHost{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})

	t.Run("podman list error on linux runs natively", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{listErr: errors.New("no machine support")}, host: fakeHost{}}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})

	// podman that cannot run (container.binary pins it on a host without it)
	// is not a machine that is down, so callers can report it as no engine.
	t.Run("podman list error on mac is passed through, not ErrMachineNotRunning", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		notFound := errors.New(`exec: "podman": executable file not found in $PATH`)
		m := &Manager{engine: Podman, podman: &fakePodman{listErr: notFound}, host: mac}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, notFound)
		assert.NotErrorIs(t, err, ErrMachineNotRunning)
	})

	t.Run("podman inspect error is passed through, not ErrMachineNotRunning", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		broken := errors.New("inspect failed")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{running("m")}, inspectErr: broken}, host: mac}
		_, err := m.ConnectionEnv()
		require.ErrorIs(t, err, broken)
		assert.NotErrorIs(t, err, ErrMachineNotRunning)
	})

	t.Run("pre-set DOCKER_HOST is respected", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "tcp://example:1234")
		m := &Manager{engine: Podman, podman: &fakePodman{listErr: errors.New("must not be called")}, host: mac}
		env, err := m.ConnectionEnv()
		require.NoError(t, err)
		assert.Nil(t, env)
	})
}

func TestInitializeStartsNothing(t *testing.T) {
	t.Run("a running machine is enough", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{running("m")}}, host: fakeHost{mac: true}}
		require.NoError(t, m.Initialize())
	})

	t.Run("no running machine is reported, not started", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		m := &Manager{engine: Podman, podman: &fakePodman{machines: []ListedMachine{{Name: "m"}}}, host: fakeHost{mac: true}}
		err := m.Initialize()
		require.ErrorIs(t, err, ErrMachineNotRunning)
		assert.Contains(t, err.Error(), "podman machine start")
	})
}
