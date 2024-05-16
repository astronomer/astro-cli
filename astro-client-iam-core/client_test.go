package astroiamcore

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestIamCoreClient(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestNew() {
	client := NewIamCoreClient(httputil.NewHTTPClient())
	s.NotNil(client, "Can't create new Astro IAM Core client")
}
