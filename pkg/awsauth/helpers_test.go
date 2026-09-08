package awsauth

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// link builds a one-link set and returns that link's instance, so a test reads
// as the manifest a user would write.
func link(t *testing.T, body string) instances.Instance {
	t.Helper()
	set := instances.Build(instancestest.Manifest(t, body))
	return instancestest.OneLink(t, set.All(), set.Names())
}

// transportVia resolves through the core's dispatch with this door wired, which
// is what a real command does. Most tests here call mwaaTransport directly;
// these few are about what the dispatch does with the options, so they go the
// long way.
func transportVia(ctx context.Context, i instances.Instance, o Options) (airflowapi.Transport, error) {
	return i.Transport(ctx, instances.Deps{
		Providers: instances.Providers{manifest.AuthAWS: Provider(o)},
	})
}
