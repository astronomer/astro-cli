package googleauth

import (
	"context"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// Options are the seams the door resolves through. The zero value asks Google's
// own chain, which is what a real run wants; a test supplies its own so it can
// answer without an account.
type Options struct {
	// Token hands back an Application Default Credentials access token, and
	// Account names the principal it speaks for — the fact that turns a
	// Composer 403 from a shrug into a fix. nil on either asks Google's chain.
	Token   func(ctx context.Context) (string, error)
	Account func(ctx context.Context) string
}

// Provider is the entry to give instances.Providers for the google method.
//
// The options are captured here, which is the load-bearing part: a Composer
// command resolves the environment's URL through this same chain, and handing
// the resolved chain on is what stops one run walking two of them.
func Provider(o Options) instances.Provider {
	return instances.Provider{
		Credentials: func(context.Context, instances.Instance, string, instances.Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
			source, refresh := googleCredentials(o)
			return source, refresh, nil
		},
	}
}
