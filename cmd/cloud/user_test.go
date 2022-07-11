package cloud

import (
	"bytes"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func execUserCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newUserCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestUserRootCommand(t *testing.T) {
	expectedHelp := "Invite Users to your Astro Organization."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newUserCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), expectedHelp)
}

func TestUserInvite(t *testing.T) {
	expectedHelp := "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER]"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints invite help", func(t *testing.T) {
		cmdArgs := []string{"invite", "-h"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("valid email with no role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role ORGANIZATION_MEMBER created"

		cmdArgs := []string{"invite", "some@email.com"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("valid email with valid role creates an invite", func(t *testing.T) {
		expectedOut := "invite for some@email.com with role mytestrole created"

		cmdArgs := []string{"invite", "some@email.com", "--role", "mytestrole"}
		resp, err := execUserCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}
