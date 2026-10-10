package cmd

import (
	"os"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
)

// contextPlatform says whether the current context is Astro Private Cloud,
// for what astro init and every hint about a 1.x project say, and whether it
// cannot be told at all:
//
//   - ASTRO_DOMAIN naming an Astro domain: Astro, whatever the home config
//     says or whether it can be read, since ASTRO_DOMAIN decides when set;
//   - no context named, by the home config or ASTRO_DOMAIN: Astro, as on a
//     fresh machine;
//   - a context that resolves: APC unless its domain is Astro's
//     (context.IsCloudDomain, as context.IsCloudContext decides it);
//   - a home config that cannot be read, or a named context the config does
//     not hold for a domain that is not Astro's: unresolved.
//
// Unresolved is not Astro, which is where context.IsCloudContext sends it:
// converting a 1.x project that deploys to APC is the mistake astro init
// refuses, so it refuses until the context is fixed or switched.
func contextPlatform() (apc, unresolved bool) {
	if d := os.Getenv("ASTRO_DOMAIN"); d != "" && context.IsCloudDomain(d) {
		return false, false
	}
	if config.HomeConfigUnreadable() {
		return false, true
	}
	domain, err := config.GetCurrentDomain()
	if err != nil {
		return false, false
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		return false, !context.IsCloudDomain(domain)
	}
	return !context.IsCloudDomain(c.Domain), false
}
