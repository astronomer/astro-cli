package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A first run guesses that the login is a new account, which opens the sign-up
// screen and preselects creating an organization. A run that may not ask
// cannot confirm the guess, so only an explicit --signup says so there.
func TestFirstRunSignupGuessNeedsARunThatMayAsk(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial)
	prev := signup
	t.Cleanup(func() { signup = prev })
	signup = false

	domain := domainutil.DefaultDomain
	if !assert.True(t, signupForDomain(domain), "a first run that may ask guesses sign-up") {
		return
	}
	t.Cleanup(input.SetGuard(func() string { return "with CI set it cannot" }))
	assert.False(t, signupForDomain(domain), "a run that may not ask does not guess")
	signup = true
	assert.True(t, signupForDomain(domain), "--signup still says so")
}
