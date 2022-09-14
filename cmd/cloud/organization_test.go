package cloud

import (
	"bytes"
	"io"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/organization"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

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
	orgList = func(out io.Writer) error {
		return nil
	}

	cmdArgs := []string{"list"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestOrganizationSwitch(t *testing.T) {
	orgSwitch = func(orgName string, client astro.Client, out io.Writer) error {
		return nil
	}

	cmdArgs := []string{"switch"}
	_, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
}
