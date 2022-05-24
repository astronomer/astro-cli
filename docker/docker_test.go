package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

func TestExecPipe(t *testing.T) {
	var buf bytes.Buffer
	data := ""
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(data))}
	err := ExecPipe(*resp, &buf, &buf, &buf)
	fmt.Println(buf.String())
	if err != nil {
		t.Error(err)
	}
}

func TestAirflowCommand(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		out := AirflowCommand("test-id", "-f docker_image_test.go")
		assert.Empty(t, out)
	})
}
