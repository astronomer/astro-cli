package deployment

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/messages"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestUserList(t *testing.T) {
	testUtil.InitTestConfig()
	// Test that UserList returns a single deployment user correctly
	okResponse := `{
		"data": {
			"deploymentUsers": [
				{
					"id": "ckgqw2k2600081qc90nbamgno",
					"fullName": "Some Person",
					"username": "somebody@astronomer.io",
					"roleBindings": [
						{
							"role": "SYSTEM_ADMIN"
						},
						{
							"role": "WORKSPACE_ADMIN"
						},
						{
							"role": "WORKSPACE_VIEWER"
						},
						{
							"role": "DEPLOYMENT_ADMIN"
						}
					]
				}
			]
		}
	}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	deploymentId := "ckgqw2k2600081qc90nbamgno"
	email := "somebody@astronomer.io"
	userId := "somebody@astronomer.io"
	fullName := "Some Person"

	buf := new(bytes.Buffer)
	err := UserList(deploymentId, email, userId, fullName, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person     somebody@astronomer.io     DEPLOYMENT_ADMIN`)

	// Test that UserList returns a list of deployment users correctly
	okResponse = `{
		"data": {
			"deploymentUsers": [
				{
					"id": "ckgqw2k2600081qc90nbamgno",
					"fullName": "Some Person",
					"username": "somebody@astronomer.io",
					"roleBindings": [
						{
							"role": "SYSTEM_ADMIN"
						},
						{
							"role": "WORKSPACE_ADMIN"
						},
						{
							"role": "WORKSPACE_VIEWER"
						},
						{
							"role": "DEPLOYMENT_ADMIN"
						}
					]
				},
				{
					"id": "ckgqw2k2600081qc90nbamgno",
					"fullName": "Another Person",
					"username": "somebody+1@astronomer.io",
					"roleBindings": [
						{
							"role": "WORKSPACE_VIEWER"
						},
						{
							"role": "DEPLOYMENT_EDITOR"
						}
					]
				}
			]
		}
	}
`
	client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api = astrohub.NewAstrohubClient(client)
	deploymentId = "ckgqw2k2600081qc90nbamgno"

	buf = new(bytes.Buffer)
	err = UserList(deploymentId, "", "", "", api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgno     Some Person        somebody@astronomer.io       DEPLOYMENT_ADMIN`)
	assert.Contains(t, buf.String(), `ckgqw2k2600081qc90nbamgno     Another Person     somebody+1@astronomer.io     DEPLOYMENT_EDITOR`)

	// Test that UserList returns an empty list when deployment does not exist
	// TODO: @adam2k update this test with the correct error response once the Houston API work is completed
	okResponse = `{
		"data": {
			"deploymentUsers": []
		}
	}
`
	client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api = astrohub.NewAstrohubClient(client)
	deploymentId = "ckgqw2k2600081qc90nbamgno"

	buf = new(bytes.Buffer)
	err = UserList(deploymentId, "", "", "", api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), messages.HoustonInvalidDeploymentUsers)
}

func TestAdd(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"deploymentAddUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_ADMIN",
				"deployment": {
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	deploymentId := "ckggzqj5f4157qtc9lescmehm"
	email := "somebody@astronomer.com"
	role := "DEPLOYMENT_ADMIN"

	buf := new(bytes.Buffer)
	err := Add(deploymentId, email, role, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully added somebody@astronomer.com as a DEPLOYMENT_ADMIN")

	errResponse := `{
		"errors": [
			{
				"message": "A duplicate role binding already exists",
				"locations": [
					{
						"line": 2,
						"column": 3
					}
				],
				"path": [
					"deploymentAddUserRole"
				],
				"extensions": {
					"code": "BAD_USER_INPUT",
					"exception": {
						"message": "A duplicate role binding already exists",
						"stacktrace": [
							"UserInputError: A duplicate role binding already exists"
						]
					}
				}
			}
		],
		"data": {
			"deploymentAddUserRole": null
		}
	}
	`

	client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(errResponse)),
			Header:     make(http.Header),
		}
	})
	api = astrohub.NewAstrohubClient(client)
	deploymentId = "ckggzqj5f4157qtc9lescmehm"
	email = "somebody@astronomer.com"
	role = "DEPLOYMENT_ADMIN"

	buf = new(bytes.Buffer)
	err = Add(deploymentId, email, role, api, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "A duplicate role binding already exists")
}

func TestDeleteUser(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"deploymentRemoveUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_ADMIN",
				"deployment": {
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	deploymentId := "ckggzqj5f4157qtc9lescmehm"
	email := "somebody@astronomer.com"

	buf := new(bytes.Buffer)
	err := DeleteUser(deploymentId, email, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully removed the DEPLOYMENT_ADMIN role for somebody@astronomer.com from deployment ckggzqj5f4157qtc9lescmehm")

	// Test that an error message appears when a user is not deleted
	errResponse := `{
		"errors": [
			{
				"message": "The role binding does not exist for this user",
				"locations": [
					{
						"line": 2,
						"column": 3
					}
				],
				"path": [
					"deploymentRemoveUserRole"
				],
				"extensions": {
					"code": "BAD_USER_INPUT",
					"exception": {
						"message": "The role binding does not exist for this user",
						"stacktrace": [
							"UserInputError: The role binding does not exist for this user"
						]
					}
				}
			}
		],
		"data": {
			"deploymentRemoveUserRole": null
		}
	}
`

	client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 400,
			Body:       ioutil.NopCloser(bytes.NewBufferString(errResponse)),
			Header:     make(http.Header),
		}
	})
	api = astrohub.NewAstrohubClient(client)
	deploymentId = "ckggzqj5f4157qtc9lescmehm"
	email = "somebody@astronomer.com"

	buf = new(bytes.Buffer)
	err = DeleteUser(deploymentId, email, api, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The role binding does not exist for this user")
}

func TestUpdateUser(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"deploymentUpdateUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_EDITOR",
				"deployment": {
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := astrohub.NewAstrohubClient(client)
	deploymentId := "ckggzqj5f4157qtc9lescmehm"
	email := "somebody@astronomer.com"
	role := astrohub.DeploymentEditor

	buf := new(bytes.Buffer)
	err := UpdateUser(deploymentId, email, role, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully updated somebody@astronomer.com to a DEPLOYMENT_EDITOR")

	errResponse := `{
		"error": {
			"errors": [
				{
					"message": "Variable \"$role\" got invalid value \"DEPLOYMENT_FAKE_ROLE\"; Expected type Role. Did you mean DEPLOYMENT_ADMIN, DEPLOYMENT_EDITOR, or DEPLOYMENT_VIEWER?",
					"locations": [
						{
							"line": 1,
							"column": 85
						}
					],
					"extensions": {
						"code": "INTERNAL_SERVER_ERROR",
						"exception": {
							"stacktrace": [
								"GraphQLError: Variable \"$role\" got invalid value \"DEPLOYMENT_FAKE_ROLE\"; Expected type Role. Did you mean DEPLOYMENT_ADMIN, DEPLOYMENT_EDITOR, or DEPLOYMENT_VIEWER?"
							]
						}
					}
				}
			]
		}
	}
	`

	client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 400,
			Body:       ioutil.NopCloser(bytes.NewBufferString(errResponse)),
			Header:     make(http.Header),
		}
	})
	api = astrohub.NewAstrohubClient(client)
	deploymentId = "ckggzqj5f4157qtc9lescmehm"
	email = "somebody@astronomer.com"
	role = "DEPLOYMENT_FAKE_ROLE"

	buf = new(bytes.Buffer)
	err = UpdateUser(deploymentId, email, role, api, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Expected type Role. Did you mean DEPLOYMENT_ADMIN, DEPLOYMENT_EDITOR, or DEPLOYMENT_VIEWER")
}
