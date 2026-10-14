package cmd

import (
	"os"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/project"
)

// contextPlatform says whether the current context is Astro Private Cloud,
// for what astro init and every hint about a 1.x project say, and whether it
// cannot be told at all:
//
//   - ASTRO_DOMAIN naming an Astro domain: Astro, whatever the home config
//     says or whether it can be read, since ASTRO_DOMAIN decides when set;
//   - a home config that cannot be read: unresolved, naming the file, since
//     the advice is then to fix it rather than to change the context;
//   - no context named, by the home config or ASTRO_DOMAIN: Astro, as on a
//     fresh machine;
//   - a named context the config holds: APC unless its domain is Astro's
//     (context.IsCloudDomain, as context.IsCloudContext decides it);
//   - one it does not hold, for a domain that is not Astro's: unresolved.
//
// Unresolved is not Astro, which is where context.IsCloudContext sends it:
// converting a 1.x project that deploys to APC is the mistake astro init
// refuses, so it refuses until the context is fixed or switched.
//
// It reads the home config only: which domain is current, and whether a
// context for it is saved (config.Context.ContextExists). It never reads a
// login, so it never reaches the keyring or the secrets vault, as
// context.GetCurrentContext does.
func contextPlatform() project.Context {
	env := os.Getenv("ASTRO_DOMAIN")
	if env != "" && context.IsCloudDomain(env) {
		return project.Context{}
	}
	if config.HomeConfigUnreadable() {
		return project.Context{Unresolved: true, UnreadableConfig: config.HomeConfigFile}
	}
	domain, err := config.GetCurrentDomain()
	if err != nil {
		return project.Context{}
	}
	c := project.Context{FromASTRODomain: env != ""}
	if c.FromASTRODomain {
		saved := config.CFG.Context.GetHomeString()
		c.UnsetIsAstro = saved == "" || context.IsCloudDomain(saved)
	}
	if !(&config.Context{Domain: domain}).ContextExists() {
		c.Unresolved = !context.IsCloudDomain(domain)
		return c
	}
	c.APC = !context.IsCloudDomain(domain)
	return c
}
