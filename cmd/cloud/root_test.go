package cloud

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestAddCmds(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmds := AddCmds(nil, nil, nil, nil, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy DEPLOYMENT-ID", "env", "workspace", "user", "organization", "dbt", "ide", "remote"}, cmds[cmdIdx].Use)
	}
}
