package cloud

import (
	"bytes"
	"io"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

//nolint:unparam
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
	orgList = func(out io.Writer, coreClient astrocore.CoreClient) error {
		return nil
	}

	cmdArgs := []string{"list"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationSwitch(t *testing.T) {
	orgSwitch = func(orgName string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
		return nil
	}

	cmdArgs := []string{"switch"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationExportAuditLogs(t *testing.T) {
	orgExportAuditLogs = func(client astro.Client, out io.Writer, orgName string, earliest int) error {
		return nil
	}

	t.Run("Fails without organization name", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.Contains(t, err.Error(), "required flag(s) \"organization-name\" not set")
	})

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
}
