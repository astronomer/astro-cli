package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/docker/docker/api/types"
	"strings"
	"testing"
)

func TestExecVersion(t *testing.T) {
	err := Exec("version")
	if err != nil {
		t.Error(err)
	}
}

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
