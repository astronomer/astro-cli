package cloud

import (
	"bytes"
	"testing"

	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/stretchr/testify/assert"
)

func TestAddCmds(t *testing.T) {
	astroMock := new(astro_mocks.Client)
	buf := new(bytes.Buffer)
	cmds := AddCmds(astroMock, nil, nil, nil, buf)
	for cmdIdx := range cmds {
		assert.Contains(t, []string{"deployment", "deploy DEPLOYMENT-ID", "workspace", "user", "organization"}, cmds[cmdIdx].Use)
	}
	astroMock.AssertExpectations(t)
}
