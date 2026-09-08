package awsauth

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// Options are the seams the door resolves through. The zero value asks the AWS
// SDK's own chain, which is what a real run wants; a test supplies its own so
// it can answer without an account.
type Options struct {
	// Config loads the AWS SDK's configuration for a region: the credential
	// chain, and the region itself when the manifest names none. nil uses the
	// SDK's own loader, which is the whole chain — environment, profile, SSO,
	// credential_process, instance role.
	//
	// This was a field on instances.Deps, and moving it is what let the core
	// stop naming aws.Config: a core type mentioning it pulls the SDK into
	// every consumer, whether or not that consumer performs the method.
	Config func(ctx context.Context, region string) (aws.Config, error)
	// HTTPClient carries the web-login exchange, which needs a cookie jar the
	// airflowapi options cannot install. nil falls back to the client on
	// instances.Deps, and then to the shared default.
	HTTPClient *http.Client
}

// Provider is the entry to give instances.Providers for the aws method.
//
// A Transport rather than a credential: MWAA's requests travel inside a signed
// AWS API call, so there is no Airflow URL to address and nothing to put an
// Authorization header on.
func Provider(o Options) instances.Provider {
	return instances.Provider{
		Transport: func(ctx context.Context, i instances.Instance, d instances.Deps) (airflowapi.Transport, error) {
			// A copy per call, never the captured value. A closure captures the
			// variable rather than a snapshot, so assigning to o here would
			// latch the first caller's client for the life of this Provider and
			// race between concurrent calls — invisible in a CLI that rebuilds
			// Deps per command, and exactly wrong in a long-lived process that
			// builds one provider set and serves concurrent requests from it.
			opts := o
			// The caller's client, when this option does not override it. The
			// door used to read it straight off Deps, and dropping that made
			// Deps.HTTPClient silently stop reaching the web-login exchange.
			if opts.HTTPClient == nil {
				opts.HTTPClient = d.HTTPClient
			}
			return mwaaTransport(ctx, i, opts)
		},
	}
}

// httpClient is the client the web-login fallback rides, before its cookie jar
// is added.
func (o Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return airflowapi.DefaultHTTPClient()
}
