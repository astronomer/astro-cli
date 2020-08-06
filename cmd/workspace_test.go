package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/houston"
	"github.com/stretchr/testify/assert"
)

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := " NAME           ID                            \n" +
		"\x1b[1;32m airflow        ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n " +
		"airflow123     XXXXXXXXXXXXXXX               \n"

	okResponse := `{"data":{
    "workspaces": [
      {
        "id": "ck05r3bor07h40d02y2hw4n4v",
        "label": "airflow",
        "description": "test description"
      },
      {
        "id": "XXXXXXXXXXXXXXX",
        "label": "airflow123",
        "description": "test description 123"
      }
    ]
  }
}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "workspace", "list")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output, err)
}

func TestWorkspaceSaRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("workspace", "service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro workspace service-account")
}

func TestNewWorkspaceUserListCmd(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserListCmd(api, buf)
	assert.NotNil(t, cmd)
	assert.Nil(t, cmd.Args)
}
