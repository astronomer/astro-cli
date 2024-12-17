package houston

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func SetupSuite() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
}

func TestHouston(t *testing.T) {
	suite.Run(t, new(Suite))
}
