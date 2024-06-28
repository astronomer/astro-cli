package cloud

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddCmds(t *testing.T) {
	buf := new(bytes.Buffer)
	cmds := AddCmds(nil, nil, nil, nil, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy DEPLOYMENT-ID", "workspace", "user", "organization", "dbt"}, cmds[cmdIdx].Use)
	}
}
