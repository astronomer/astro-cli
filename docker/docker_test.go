package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestDocker(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestExecPipe() {
	var buf bytes.Buffer
	data := ""
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(data))}
	err := ExecPipe(*resp, &buf, &buf, &buf)
	fmt.Println(buf.String())
	s.NoError(err)
}

func (s *Suite) TestAirflowCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("success", func() {
		err := config.CFG.DockerCommand.SetHomeString("docker")
		s.NoError(err)
		out, err := AirflowCommand("test-id", "-f docker_image_test.go")
		s.NoError(err)
		s.Empty(out)
	})

	s.Run("error", func() {
		err := config.CFG.DockerCommand.SetHomeString("")
		s.NoError(err)
		err = os.Setenv("PATH", "") // set PATH to empty string to force error on container runtime check
		s.NoError(err)
		out, err := AirflowCommand("test-id", "-f docker_image_test.go")
		s.Error(err)
		s.Empty(out)
	})
}
