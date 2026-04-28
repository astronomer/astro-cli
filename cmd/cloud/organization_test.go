package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

//nolint:unparam
func execOrganizationCmd(args ...string) (string, error) {
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestOrganizationRootCommand(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "organization")
}

func TestOrganizationList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockOrganizationProduct := astrov1.OrganizationProductHYBRID
	mockOrgsResponse := astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
			},
		},
	}

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-org")
	mockV1Client.AssertExpectations(t)
}

func TestOrganizationListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockOrganizationProduct := astrov1.OrganizationProductHYBRID
	mockOrgsResponse := astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
			},
		},
	}

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "--json"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)

	var result organization.OrganizationList
	assert.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Len(t, result.Organizations, 1)
	assert.Equal(t, "test-org", result.Organizations[0].Name)
	assert.Equal(t, "test-org-id", result.Organizations[0].ID)
	mockV1Client.AssertExpectations(t)
}

func TestOrganizationSwitch(t *testing.T) {
	t.Run("workspace flag triggers wsSwitch with provided id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		origOrgSwitch := orgSwitch
		orgSwitch = func(orgName string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		defer func() { orgSwitch = origOrgSwitch }()

		called := false
		gotID := ""
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrov1.APIClient, out io.Writer) error {
			called = true
			gotID = id
			return nil
		}
		defer func() { wsSwitch = origWsSwitch }()

		cmdArgs := []string{"switch", "-w", "ws-test-id"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, "ws-test-id", gotID)
	})

	t.Run("orgSwitch error propagates and wsSwitch not called", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		expectedErr := fmt.Errorf("org switch failed")

		origOrgSwitch := orgSwitch
		orgSwitch = func(orgName string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return expectedErr
		}
		defer func() { orgSwitch = origOrgSwitch }()

		calledWs := false
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrov1.APIClient, out io.Writer) error {
			calledWs = true
			return nil
		}
		defer func() { wsSwitch = origWsSwitch }()

		_, err := execOrganizationCmd("switch", "-w", "ws-test-id")
		assert.ErrorIs(t, err, expectedErr)
		assert.False(t, calledWs)
	})
}

func TestOrganizationExportAuditLogs(t *testing.T) {
	// turn on audit logs
	config.CFG.AuditLogs.SetHomeString("true")
	orgExportAuditLogs = func(astroV1Client astrov1.APIClient, orgName, filePath string, earliest int) error {
		return nil
	}

	t.Run("Without params", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("with auditLogsOutputFilePath param", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer", "--output-file", "test.json"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	// Delete audit logs exports
	currentDir, _ := os.Getwd()
	files, _ := os.ReadDir(currentDir)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "audit-logs-") {
			os.Remove(file.Name())
		}
	}
	os.Remove("test.json")
}
