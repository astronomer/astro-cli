// Package containercfg is the core tree's one read of the container.binary
// setting, so cmd/local can hand it to pkg/localrt and internal/pack as a seam
// without importing config/ itself. Binary is the CLI's half of
// localrt.Config.ContainerBinary; Astro Desktop fills the same field from its
// own reader of the same files.
package containercfg

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// Binary returns container.binary from the global config, "" when unset,
// which pkg/container takes as auto-detect. It takes the project path because
// that is the seam's shape (localrt.Config.ContainerBinary and the engine
// funcs below are per project), but no project chooses the binary: which
// container engine runs is this machine's setting, and a project's
// .astro/config.yaml is never read for it.
func Binary(_ string) string {
	return config.CFG.DockerCommand.GetString()
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

// ImageEngine is Engine for the project at projectPath as the pkg/imagebuild
// engine an image's actions run on, over the exec runner: its build, its
// registry login, its tag and push, and reading its labels. A caller that
// takes one engine here for all of them never builds on one engine and pushes
// from another.
func ImageEngine(projectPath string) (imagebuild.Engine, error) {
	bin, env, err := Engine(projectPath)
	if err != nil {
		return imagebuild.Engine{}, err
	}
	return imagebuild.Engine{Cmd: imagebuild.NewExecCommander(), Bin: bin, Env: env}, nil
}
