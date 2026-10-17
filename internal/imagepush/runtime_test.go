package imagepush

import (
	"io"

	"github.com/astronomer/astro-cli/config"
)

// ContainerRuntimeBinary is the engine the login, the tag and the push run
// on: the remote-execution client deploy builds on it so that all of them,
// its build included, reach the same engine.
func (s *Suite) TestContainerRuntimeBinaryIsWhatLoginTagAndPushRunOn() {
	for _, bin := range []string{"docker", "podman"} {
		s.Run(bin, func() {
			s.Require().NoError(config.CFG.DockerCommand.SetHomeString(bin))
			got, err := ContainerRuntimeBinary()
			s.Require().NoError(err)
			s.Equal(bin, got)

			var ran []string
			cmdExec = func(cmd string, _, _ io.Writer, _ ...string) error {
				ran = append(ran, cmd)
				return errMockDocker // stop the push after its tag
			}
			registryLogin = func(containerRuntime, _, _, _ string, _, _ io.Writer) error {
				ran = append(ran, containerRuntime)
				return nil
			}
			handler := DockerImageInit("local")
			s.Require().NoError(DockerLogin("registry.example.com", "cli", "token"))
			s.Require().Error(handler.TagLocalImage("other"))
			_, err = handler.Push("registry.example.com/client:deploy-1", "", "", false)
			s.Require().ErrorIs(err, errMockDocker)
			s.Equal([]string{got, got, got}, ran, "login, tag and push")
		})
	}
}
