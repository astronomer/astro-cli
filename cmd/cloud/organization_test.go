package cloud

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
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

func TestOrganizationSwitch(t *testing.T) {
	t.Run("workspace flag triggers wsSwitch with provided id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		origOrgSwitch := orgSwitch
		orgSwitch = func(orgName string, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		defer func() { orgSwitch = origOrgSwitch }()

		called := false
		gotID := ""
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrocore.CoreClient, out io.Writer) error {
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
		orgSwitch = func(orgName string, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return expectedErr
		}
		defer func() { orgSwitch = origOrgSwitch }()

		calledWs := false
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrocore.CoreClient, out io.Writer) error {
			calledWs = true
			return nil
		}
		defer func() { wsSwitch = origWsSwitch }()

		_, err := execOrganizationCmd("switch", "-w", "ws-test-id")
		assert.ErrorIs(t, err, expectedErr)
		assert.False(t, calledWs)
	})
}
