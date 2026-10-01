// Package containercfg is the v2 tree's one read of the container.binary
// setting, so cmd/local can hand it to pkg/localrt and internal/pack as a seam
// without importing config/ itself. Binary is the CLI's half of
// localrt.Config.ContainerBinary; Astro Desktop fills the same field from its
// own reader of the same files.
package containercfg

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/container"
)

// Binary returns container.binary for the project at projectPath: the
// project's .astro/config.yaml when it sets one (a v1 project; a v2 project
// has no such file), the global config otherwise. "" means unset, which
// pkg/container takes as auto-detect. An empty projectPath asks for the global
// value.
func Binary(projectPath string) string {
	return config.CFG.DockerCommand.GetStringFor(projectPath)
}

// Engine resolves the container CLI for the project at projectPath — the
// container.binary pin when set, otherwise pkg/container's PATH/OrbStack
// detection — and the env that reaches its daemon (a podman machine's
// socket; nil for docker). It errors when no supported engine is found, and
// with container.ErrMachineNotRunning — wrapped with the podman command that
// fixes it — when podman needs a machine and none is up, so a caller can say
// that instead of "start Docker". An engine that is merely down is left to the
// probe each caller runs next.
func Engine(projectPath string) (bin string, env []string, err error) {
	eng, err := container.Resolve(container.Config{Binary: Binary(projectPath)})
	if err != nil {
		return "", nil, err
	}
	bin = eng.Binary()
	mgr, err := container.NewManager(container.Config{Binary: bin}, nil)
	if err != nil {
		return bin, nil, nil
	}
	if env, err = mgr.ConnectionEnv(); err != nil {
		return "", nil, err
	}
	return bin, env, nil
}
