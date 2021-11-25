package workspace

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
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
	api := houston.NewHoustonClient(client)
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
	assert.Equal(t, expected, buf.String())
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
	api := houston.NewHoustonClient(client)
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
	api := houston.NewHoustonClient(client)
	id := "ck1qg6whg001r08691y117hub"
	userID := "ckc0eir8e01gj07608ajmvia1"

	buf := new(bytes.Buffer)
	err := Remove(id, userID, api, buf)
	assert.NoError(t, err)
	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
                               ckc0eir8e01gj07608ajmvia1                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`
	assert.Equal(t, expected, buf.String())
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
	api := houston.NewHoustonClient(client)
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
	api := houston.NewHoustonClient(client)
	wsID := "ck1qg6whg001r08691y117hub"

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.NoError(t, err)
	expected := ` USERNAME                 ID                            ROLE                
 andrii@astronomer.io     ckbv7zpkh00og0760ki4mhl6r     WORKSPACE_ADMIN     
`
	assert.Equal(t, expected, buf.String())
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
	api := houston.NewHoustonClient(client)
	wsID := "ck1qg6whg001r08691y117hub"

	buf := new(bytes.Buffer)
	err := ListRoles(wsID, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestUpdateRole(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data":{"workspaceUpdateUserRole":"WORKSPACE_ADMIN", "workspaceUser":{"roleBindings":[{"workspace":{"id":"ckg6sfddu30911pc0n1o0e97e"},"role":"DEPLOYMENT_VIEWER"},{"workspace":{"id":"ckoixo6o501496qemiwsja1tl"},"role":"WORKSPACE_VIEWER"},{"workspace":{"id":"ckg6sfddu30911pc0n1o0e97e"},"role":"WORKSPACE_EDITOR"},{"workspace":{"id":"ckql4ias908766qbpuck803fa"},"role":"WORKSPACE_ADMIN"}]}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.NoError(t, err)
	expected := `Role has been changed from WORKSPACE_VIEWER to WORKSPACE_ADMIN for user test-role`
	assert.Equal(t, expected, buf.String())
}

func TestUpdateRoleNoAccessDeploymentOnly(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data":{"workspaceUpdateUserRole":"WORKSPACE_ADMIN", "workspaceUser":{"roleBindings":[{"workspace":{"id":"ckg6sfddu30911pc0n1o0e97e"},"role":"DEPLOYMENT_VIEWER"}]}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ckg6sfddu30911pc0n1o0e97e"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
}

func TestUpdateRoleNoAccess(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data":{"workspaceUpdateUserRole":"WORKSPACE_ADMIN", "workspaceUser":{"roleBindings":[]}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ckoixo6o501496qemiwsja1tl"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.Equal(t, "the user you are trying to change is not part of this workspace", err.Error())
}

func TestUpdateRoleError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	email := "andrii@test.com"

	buf := new(bytes.Buffer)
	err := UpdateRole(id, role, email, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestGetUserRole(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data":{    "appConfig": {"nfsMountDagDeployment": false},"workspaceUser":{"roleBindings":[{"workspace":{"id":"ckg6sfddu30911pc0n1o0e97e"},"role":"DEPLOYMENT_VIEWER"},{"workspace":{"id":"ckoixo6o501496qemiwsja1tl"},"role":"WORKSPACE_VIEWER"},{"workspace":{"id":"ckg6sfddu30911pc0n1o0e97e"},"role":"WORKSPACE_EDITOR"},{"workspace":{"id":"ckql4ias908766qbpuck803fa"},"role":"WORKSPACE_ADMIN"}]}}}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ck1qg6whg001r08691y117hub"
	email := "andrii@test.com"

	userRoleBinding, err := getUserRole(id, email, api)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(userRoleBinding.RoleBindings))
}

func TestGetUserRoleError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ck1qg6whg001r08691y117hub"
	email := "andrii@test.com"

	_, err := getUserRole(id, email, api)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}
