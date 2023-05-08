package astrocore

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestAirflowClientSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestNewCoreClient() {
	client := NewCoreClient(httputil.NewHTTPClient())
	s.NotNil(client, "Can't create new Astro Core client")
}
