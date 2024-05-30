package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

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
	s.Run("success", func() {
		out := AirflowCommand("test-id", "-f docker_image_test.go")
		s.Empty(out)
	})
}
