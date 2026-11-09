package user

import (
	"context"

	houstonMocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
)

// An interrupt at the password prompt stops the create: no user is created
// with an empty password (the mock fails on any call).
func (s *Suite) TestCreateStopsOnInterrupt() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.T().Cleanup(input.SetInterrupt(ctx))

	_, err := Create("test@test.com", "", new(houstonMocks.ClientInterface))
	s.ErrorIs(err, input.ErrInterrupted)
}
