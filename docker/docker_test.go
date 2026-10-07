package docker

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestDocker(t *testing.T) {
	suite.Run(t, new(Suite))
}
