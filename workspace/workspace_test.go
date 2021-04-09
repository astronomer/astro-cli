package workspace

import (
	"bytes"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/astrohub"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{
  "data": {
    "createWorkspace": {
      "id": "ckc0j8y1101xo0760or02jdi7",
      "label": "test",
      "description": "test"
    }
  }
}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	label := "test"
	description := "description"

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     ID                            
 test     ckc0j8y1101xo0760or02jdi7     

 Successfully created workspace
`
	assert.Equal(t, buf.String(), expected)
}

func TestCreateError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	label := "test"
	description := "description"

	buf := new(bytes.Buffer)
	err := Create(label, description, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestList(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{
  "data": {
    "workspaces": [
      {
        "id": "ckbv7zvb100pe0760xp98qnh9",
        "label": "w1",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          },
          {
            "role": "WORKSPACE_VIEWER",
            "user": {
              "id": "ckc0eilr201fl07602i8gq4vo",
              "username": "andrii.soldatenko@gmail.com"
            }
          }
        ]
      },
      {
        "id": "ckbv8pwbq00wk0760us7ktcgd",
        "label": "wwww",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          }
        ]
      },
      {
        "id": "ckc0j8y1101xo0760or02jdi7",
        "label": "test",
        "description": "test",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          }
        ]
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
	api := astrohub.NewAstrohubClient(client)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.NoError(t, err)
	expected := ` NAME     ID                            
 w1       ckbv7zvb100pe0760xp98qnh9     
 wwww     ckbv8pwbq00wk0760us7ktcgd     
 test     ckc0j8y1101xo0760or02jdi7     
`
	assert.Equal(t, buf.String(), expected)
}

func TestListActiveWorkspace(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{
  "data": {
    "workspaces": [
      {
        "id": "ck05r3bor07h40d02y2hw4n4v",
        "label": "w1",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          },
          {
            "role": "WORKSPACE_VIEWER",
            "user": {
              "id": "ckc0eilr201fl07602i8gq4vo",
              "username": "andrii.soldatenko@gmail.com"
            }
          }
        ]
      },
      {
        "id": "ckbv8pwbq00wk0760us7ktcgd",
        "label": "wwww",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          }
        ]
      },
      {
        "id": "ckc0j8y1101xo0760or02jdi7",
        "label": "test",
        "description": "test",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          }
        ]
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
	api := astrohub.NewAstrohubClient(client)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.NoError(t, err)
	expected := " NAME     ID                            \n\x1b[1;32m w1       ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n wwww     ckbv8pwbq00wk0760us7ktcgd     \n test     ckc0j8y1101xo0760or02jdi7     \n"
	assert.Equal(t, expected, buf.String())
}

func TestListError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)

	buf := new(bytes.Buffer)
	err := List(api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{
  "data": {
    "deleteWorkspace": {
      "id": "ckc0j8y1101xo0760or02jdi7"
    }
  }
}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	wsId := "ckc0j8y1101xo0760or02jdi7"

	buf := new(bytes.Buffer)
	err := Delete(wsId, api, buf)
	assert.NoError(t, err)
	expected := "\n Successfully deleted workspace\n"
	assert.Equal(t, expected, buf.String())
}

func TestDeleteError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	wsId := "ckc0j8y1101xo0760or02jdi7"

	buf := new(bytes.Buffer)
	err := Delete(wsId, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestGetCurrentWorkspace(t *testing.T) {
	// we init default workspace to: ck05r3bor07h40d02y2hw4n4v
	testUtil.InitTestConfig()

	ws, err := GetCurrentWorkspace()
	assert.NoError(t, err)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", ws)
}

func TestGetCurrentWorkspaceError(t *testing.T) {
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, config.HomeConfigFile, []byte(""), 0777)
	config.InitConfig(fs)
	_, err = GetCurrentWorkspace()
	assert.EqualError(t, err, "No context set, have you authenticated to a cluster?")
}

func TestGetCurrentWorkspaceErrorNoCurrentContext(t *testing.T) {
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, config.HomeConfigFile, []byte(configRaw), 0777)
	config.InitConfig(fs)
	_, err = GetCurrentWorkspace()
	assert.EqualError(t, err, "Current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
}

func TestGetWorkspaceSelectionError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)

	buf := new(bytes.Buffer)
	_, err := getWorkspaceSelection(api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestSwitch(t *testing.T) {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, config.HomeConfigFile, []byte(configRaw), 0777)
	config.InitConfig(fs)

	// prepare houston-api fake response
	okResponse := `{
  "data": {
    "workspaces": [
      {
        "id": "ckbv7zvb100pe0760xp98qnh9",
        "label": "w1",
        "description": "",
        "roleBindings": [
          {
            "role": "WORKSPACE_ADMIN",
            "user": {
              "id": "ckbv7zpkh00og0760ki4mhl6r",
              "username": "andrii@astronomer.io"
            }
          },
          {
            "role": "WORKSPACE_VIEWER",
            "user": {
              "id": "ckc0eilr201fl07602i8gq4vo",
              "username": "andrii.soldatenko@gmail.com"
            }
          }
        ]
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
	api := astrohub.NewAstrohubClient(client)

	wsId := "ckbv7zvb100pe0760xp98qnh9"

	buf := new(bytes.Buffer)
	err = Switch(wsId, api, buf)
	assert.NoError(t, err)
	expected := " CLUSTER                             WORKSPACE                           \n localhost                           ckbv7zvb100pe0760xp98qnh9           \n"
	assert.Equal(t, expected, buf.String())
}

func TestSwitchHoustonError(t *testing.T) {
	// prepare test config and init it
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
context: localhost
contexts:
  localhost:
    domain: localhost
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, config.HomeConfigFile, []byte(configRaw), 0777)
	config.InitConfig(fs)

	// prepare houston-api fake response
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)

	wsId := "ckbv7zvb100pe0760xp98qnh9"

	buf := new(bytes.Buffer)
	err = Switch(wsId, api, buf)
	assert.EqualError(t, err, "workspace id is not valid: API error (500): Internal Server Error")
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig()
	okReponse := `{
  "data": {
    "updateWorkspace": {
      "id": "ckbv7zvb100pe0760xp98qnh9",
      "description": "test",
      "label": "w1"
    }
  }
}`

	// prepare houston-api fake response
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okReponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "test"
	args := map[string]string{"1": "2"}

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	assert.NoError(t, err)
	expected := " NAME     ID                            \n w1       ckbv7zvb100pe0760xp98qnh9     \n\n Successfully updated workspace\n"
	assert.Equal(t, expected, buf.String())
}

func TestUpdateError(t *testing.T) {
	testUtil.InitTestConfig()

	// prepare houston-api fake response
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "test"
	args := map[string]string{"1": "2"}

	buf := new(bytes.Buffer)
	err := Update(id, api, buf, args)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}
