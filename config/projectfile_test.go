package config

import (
	"path/filepath"

	"github.com/spf13/afero"
)

// No setting is read from a project's .astro/config.yaml: a file committed to
// a repository must not choose the container binary, a registry, TLS
// verification or an API endpoint for whoever runs the CLI there. The global
// config, and the default under it, decide every setting.
func (s *Suite) TestAProjectConfigSetsNothing() {
	fs := afero.NewMemMapFs()
	const home = "home"
	s.withAstroHome(home)
	s.NoError(afero.WriteFile(fs, filepath.Join(home, ConfigDir, ConfigFileNameWithExt), []byte(
		"config_version: 2\ncontainer:\n  binary: docker\n"), 0o600))
	s.NoError(afero.WriteFile(fs, filepath.Join(WorkingPath, ConfigDir, ConfigFileNameWithExt), []byte(
		"container:\n  binary: podman\n"+
			"remote:\n  client_registry: attacker.example/collect\n"+
			"houston:\n  skip_verify_tls: true\n"+
			"internal:\n  api:\n    protocol: http\n"), 0o600))
	s.T().Cleanup(func() { _ = fs.Remove(filepath.Join(WorkingPath, ConfigDir, ConfigFileNameWithExt)) })

	InitConfig(fs)

	s.Equal("docker", CFG.DockerCommand.GetString(), "the global value")
	s.Empty(CFG.RemoteClientRegistry.GetString(), "unset globally")
	s.False(CFG.HoustonSkipVerifyTLS.GetBool(), "the default")
	s.Equal(CFG.CloudAPIProtocol.Default, CFG.CloudAPIProtocol.GetString(), "the default")
}
