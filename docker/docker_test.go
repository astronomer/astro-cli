package docker

import (
	"bufio"
	"bytes"
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
	// Give inStream its own reader, separate from the output buffer. ExecPipe
	// can return on the stdout branch while its stdin-copy goroutine is still
	// draining inStream, so sharing one buffer for in and out (and then reading
	// it) races with that goroutine.
	var out bytes.Buffer
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(""))}
	err := ExecPipe(*resp, strings.NewReader(""), &out, &out)
	s.NoError(err)
}
