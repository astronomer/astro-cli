package deployment

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestDeployment(t *testing.T) {
	suite.Run(t, new(Suite))
}
