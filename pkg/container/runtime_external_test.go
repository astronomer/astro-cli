package container_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/container/mocks"
)

// TestMocksConformance asserts the hand-written mocks satisfy the package
// interfaces (compile-time check), and exercises NewXxx constructors so the
// mocks package is covered.
func TestMocksConformance(t *testing.T) {
	var _ container.PodmanEngine = (*mocks.PodmanEngine)(nil)
	var _ container.DockerEngine = (*mocks.DockerEngine)(nil)
	var _ container.OSChecker = (*mocks.OSChecker)(nil)

	pe := mocks.NewPodmanEngine(t)
	de := mocks.NewDockerEngine(t)
	osc := mocks.NewOSChecker(t)
	assert.NotNil(t, pe)
	assert.NotNil(t, de)
	assert.NotNil(t, osc)
}

func TestOSCheckerAndIsPodman(t *testing.T) {
	osc := container.CreateOSChecker()
	// Exactly one (or neither, on Linux) of IsMac/IsWindows is true; both are
	// deterministic booleans we just exercise here.
	_ = osc.IsMac()
	_ = osc.IsWindows()

	assert.True(t, container.IsPodman("podman"))
	assert.False(t, container.IsPodman("docker"))
}

func TestGetContainerRuntimeBinaryOverride(t *testing.T) {
	bin, err := container.GetContainerRuntimeBinary(container.Config{Binary: "podman"})
	assert.NoError(t, err)
	assert.Equal(t, "podman", bin)

	bin, err = container.GetContainerRuntimeBinary(container.Config{Binary: "docker"})
	assert.NoError(t, err)
	assert.Equal(t, "docker", bin)
}

func TestGetContainerRuntimeWithBinaryOverride(t *testing.T) {
	t.Run("podman override returns a Manager", func(t *testing.T) {
		rt, err := container.GetContainerRuntime(container.Config{Binary: "podman"}, nil)
		assert.NoError(t, err)
		_, ok := rt.(*container.Manager)
		assert.True(t, ok, "expected *Manager for podman")
	})

	t.Run("docker override returns a DockerRuntime", func(t *testing.T) {
		rt, err := container.GetContainerRuntime(container.Config{Binary: "docker"}, nil)
		assert.NoError(t, err)
		_, ok := rt.(*container.DockerRuntime)
		assert.True(t, ok, "expected *DockerRuntime for docker")
	})
}
