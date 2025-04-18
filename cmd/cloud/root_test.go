package cloud

import (
	"bytes"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestAddCmds(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmds := AddCmds(nil, nil, nil, nil, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy DEPLOYMENT-ID", "workspace", "user", "organization", "dbt", "polaris"}, cmds[cmdIdx].Use)
	}
}
