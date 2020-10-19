package deployment

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
	api := houston.NewHoustonClient(client)
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
							"UserInputError: A duplicate role binding already exists",
							"    at deploymentAddUserRole (/Users/adamvandover/Documents/GitHub/houston-api/src/resolvers/mutation/deployment-add-user-role/index.js:69:11)",
							"    at runMicrotasks (<anonymous>)",
							"    at processTicksAndRejections (internal/process/task_queues.js:97:5)",
							"    at middleware (/Users/adamvandover/Documents/GitHub/houston-api/node_modules/graphql-shield/src/generator.ts:70:16)"
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
	api = houston.NewHoustonClient(client)
	deploymentId = "ckggzqj5f4157qtc9lescmehm"
	email = "somebody@astronomer.com"
	role = "DEPLOYMENT_ADMIN"

	buf = new(bytes.Buffer)
	err = Add(deploymentId, email, role, api, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "A duplicate role binding already exists")
}
