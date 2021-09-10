package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
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

func TestExecPipeNils(t *testing.T) {
	data := ""
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(data))}
	err := ExecPipe(*resp, nil, nil, nil)
	if err != nil {
		t.Error(err)
	}
}
