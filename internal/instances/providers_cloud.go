package instances

import (
	"context"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// CloudProviders is the pair a build that talks to AWS and Google carries.
//
// Named "cloud" rather than "platform": in this repo platform already means the
// Astro and APC control planes, and archlint enforces who may import that tree,
// so a file called providers_platform.go reads as the wrong thing entirely to
// anyone holding that vocabulary. These are the two cloud vendors.
//
// The CLI wires this at both composition roots. A consumer that talks to
// neither leaves Deps.Providers zero and performs the six methods that need
// nothing. This function is the one place naming the two implementations, and
// it is what a later change lifts out of this package so the SDK chains can
// leave with it.
func CloudProviders() Providers {
	return Providers{
		manifest.AuthGoogle: {Credentials: googleProvider},
		manifest.AuthAWS:    {Transport: mwaaProvider},
	}
}

// googleProvider adapts googleCredentials, whose token source renews itself, so
// the hook it returns is about reporting a 403 rather than retrying.
func googleProvider(_ context.Context, _ Instance, _ string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
	source, refresh := googleCredentials(d)
	return source, refresh, nil
}

// mwaaProvider hands back MWAA's signed-API transport, which replaces the HTTP
// door rather than adding a credential to it.
func mwaaProvider(ctx context.Context, i Instance, d Deps) (airflowapi.Transport, error) {
	return i.mwaaTransport(ctx, d)
}
