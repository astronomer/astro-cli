package software

import (
	"bytes"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestUserRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	userCmd := newUserCmd(os.Stdout)
	userCmd.SetOut(buf)
	_, err := userCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Users represents a human who has authenticated with the Astronomer platform")
	assert.Contains(t, buf.String(), "create")
}
