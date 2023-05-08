package cloud

import (
	"bytes"

	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
)

func (s *Suite) TestAddCmds() {
	astroMock := new(astro_mocks.Client)
	buf := new(bytes.Buffer)
	cmds := AddCmds(astroMock, nil, nil, buf)
	for cmdIdx := range cmds {
		s.Contains([]string{"deployment", "deploy DEPLOYMENT-ID", "workspace", "user", "organization"}, cmds[cmdIdx].Use)
	}
	astroMock.AssertExpectations(s.T())
}
