package config

import (
	"path/filepath"

	"github.com/spf13/afero"
)

// GetStringFor reads the named project's config, not the working directory's:
// astro local acts on a project it was handed, and container.binary set in
// that project's .astro/config.yaml has to win over the global value even when
// the command runs from somewhere else.
func (s *Suite) TestGetStringForReadsTheNamedProject() {
	fs := afero.NewMemMapFs()
	const home = "home"
	s.withAstroHome(home)
	s.NoError(afero.WriteFile(fs, filepath.Join(home, ConfigDir, ConfigFileNameWithExt),
		[]byte("container:\n  binary: docker\n"), 0o600))
	s.NoError(afero.WriteFile(fs, filepath.Join("pinned", ConfigDir, ConfigFileNameWithExt),
		[]byte("container:\n  binary: podman\n"), 0o600))
	s.NoError(afero.WriteFile(fs, filepath.Join("unpinned", ConfigDir, ConfigFileNameWithExt),
		[]byte("project:\n  name: unpinned\n"), 0o600))
	initHome(fs)

	s.Equal("podman", CFG.DockerCommand.GetStringFor("pinned"), "the project's own setting")
	s.Equal("docker", CFG.DockerCommand.GetStringFor("unpinned"), "a project config without it falls back to global")
	s.Equal("docker", CFG.DockerCommand.GetStringFor("no-config-here"), "a v2 project has no .astro/config.yaml")
	s.Equal("docker", CFG.DockerCommand.GetStringFor(""), "no project asks for the global value")
}
