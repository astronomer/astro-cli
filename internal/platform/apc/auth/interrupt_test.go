package auth

import (
	"context"

	"github.com/astronomer/astro-cli/config"
	houstonMocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
)

// An interrupt at the password prompt stops the login: nothing is sent to
// Houston with an empty password (the mock fails on any call).
func (s *Suite) TestBasicAuthStopsOnInterrupt() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.T().Cleanup(input.SetInterrupt(ctx))

	_, err := basicAuth("test", "", &config.Context{}, new(houstonMocks.ClientInterface))
	s.ErrorIs(err, input.ErrInterrupted)
}
