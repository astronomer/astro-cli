package container

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHost is a host whose $PATH contains exactly the binaries in present.
type fakeHost struct {
	windows bool
	mac     bool
	present map[string]bool // binary basenames that "exist" on PATH
}

func (f fakeHost) isWindows() bool          { return f.windows }
func (f fakeHost) isMac() bool              { return f.mac }
func (f fakeHost) pathEnv() string          { return "/usr/local/bin" }
func (f fakeHost) fileExists(p string) bool { return f.present[filepath.Base(p)] }

func macHost(bins ...string) fakeHost {
	present := map[string]bool{}
	for _, b := range bins {
		present[b] = true
	}
	return fakeHost{mac: true, present: present}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		host fakeHost
		want Engine
		err  bool
	}{
		{name: "docker only", host: macHost("docker"), want: Docker},
		{name: "podman only", host: macHost("podman"), want: Podman},
		{name: "both installed prefers docker", host: macHost("docker", "podman"), want: Docker},
		{name: "binary override to podman", cfg: Config{Binary: "podman"}, host: macHost("docker", "podman"), want: Podman},
		{name: "binary override to docker", cfg: Config{Binary: "docker"}, host: macHost("docker", "podman"), want: Docker},
		{name: "orbstack via orbctl", host: macHost("docker", "orbctl"), want: Orbstack},
		{name: "ignores unsupported override then detects", cfg: Config{Binary: "nerdctl"}, host: macHost("podman"), want: Podman},
		{name: "none found", host: macHost(), err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveWith(tt.cfg, tt.host)
			if tt.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEngineBinary(t *testing.T) {
	assert.Equal(t, "docker", Docker.Binary())
	assert.Equal(t, "docker", Orbstack.Binary())
	assert.Equal(t, "podman", Podman.Binary())
}

func TestFindBinaryWindowsAppendsExe(t *testing.T) {
	h := fakeHost{windows: true, present: map[string]bool{"docker.exe": true}}
	assert.True(t, findBinary(h.pathEnv(), "docker", h))
	assert.False(t, findBinary(h.pathEnv(), "podman", h))
}
