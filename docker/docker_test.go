package docker

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
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

func TestExecPushErrAuthConfig(t *testing.T) {
	testUtil.InitTestConfig()
	err := ExecPush("", "", "")
	assert.EqualError(t, err, "Error reading credentials: error getting credentials - err: no credentials server URL, out: `no credentials server URL`")
}
