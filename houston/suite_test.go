package houston

import (
	"testing"

	"github.com/stretchr/testify/suite"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func (s *Suite) SetupSuite() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
}

func TestHouston(t *testing.T) {
	suite.Run(t, new(Suite))
}
