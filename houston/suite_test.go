package houston

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestHouston(t *testing.T) {
	suite.Run(t, new(Suite))
}
