package workspace

import (
	"bytes"
	"github.com/astronomer/astro-cli/astrohub"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestAdd(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{"data":{"workspaceAddUser":{"id":"ckc0eir8e01gj07608ajmvia1"}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     WORKSPACE ID                  EMAIL               ROLE          
          ckc0eir8e01gj07608ajmvia1     andrii@test.com     test-role     
Successfully added andrii@test.com to 
`
	assert.Equal(t, buf.String(), expected)
}

func TestAddError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := Add(id, email, role, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestRemove(t *testing.T) {
	testUtil.InitTestConfig()

	okResponse := `{"data":{"workspaceRemoveUser":{"id":"ckc0eir8e01gj07608ajmvia1"}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	userId := "ckc0eir8e01gj07608ajmvia1"

	buf := new(bytes.Buffer)
	err := Remove(id, userId, api, buf)
	assert.NoError(t, err)
	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
                               ckc0eir8e01gj07608ajmvia1                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`
	assert.Equal(t, buf.String(), expected)
}

func TestRemoveError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := Remove(id, email, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestListRoles(t *testing.T) {
	okResponse := `{
  "data": {
    "workspaces": [
      {
        "id": "ckbv7zvb100pe0760xp98qnh9",
        "label": "w1",
        "description": "",
        "createdAt": "2020-06-25T20:09:29.917Z",
        "updatedAt": "2020-06-25T20:09:29.917Z",
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
        "id": "ckbv8pwbq00wk0760us7ktcgd",
        "label": "wwww",
        "description": "",
        "createdAt": "2020-06-25T20:29:44.294Z",
        "updatedAt": "2020-06-25T20:29:44.294Z",
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
	wsId := "ck1qg6whg001r08691y117hub"

	buf := new(bytes.Buffer)
	err := ListRoles(wsId, api, buf)
	assert.NoError(t, err)
	expected := ` USERNAME                 ID                            ROLE                
 andrii@astronomer.io     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, buf.String(), expected)
}

func TestListRolesError(t *testing.T) {
	testUtil.InitTestConfig()

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	wsId := "ck1qg6whg001r08691y117hub"

	buf := new(bytes.Buffer)
	err := ListRoles(wsId, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestUpdateRole(t *testing.T) {
	okResponse := `{"data":{"workspaceUpdateUserRole":"WORKSPACE_VIEWER"}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.NoError(t, err)
	expected := `Role has been changed from andrii@test.com to WORKSPACE_VIEWER for user test-role`
	assert.Equal(t, buf.String(), expected)
}

func TestUpdateRoleError(t *testing.T) {
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}
