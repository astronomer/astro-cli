package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/organization"
	// "github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

var mockResponse = []organization.OrgRes{
	{
		AuthServiceID: "auth-service-id",
		Name:          "name",
	},
	{
		AuthServiceID: "auth-service-id-2",
		Name:          "name-2",
	},
}

func execOrganizationCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestOrganizationRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "organization")
}

func TestOrganizationList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})

	// organization.ListOrganizations = func(c config.Context) ([]organization.OrgRes, error) {
	// 	return orgResponse, nil
	// }

	cmdArgs := []string{"list"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "name")
}

func TestOrganizationSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	// organization.ListOrganizations = func(c config.Context) ([]organization.OrgRes, error) {
	// 	return orgResponse, nil
	// }
	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})

	organization.AuthLogin = func(domain, id string, client astro.Client, out io.Writer, shouldDisplayLoginLink, shouldLoginWithToken bool) error {
		return nil
	}

	// mock os.Stdin
	input := []byte("1")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	cmdArgs := []string{"switch"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "name")
}
