package registry

import (
	"bytes"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRegistryCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newRegistryCmd()
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Astronomer Registry")
}
