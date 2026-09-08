package awsauth

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The two helpers this package's tests share with internal/instances'.
//
// Copied rather than shared: a helper package existing only so two test suites
// can build an Instance from TOML would be a third thing to keep in step with
// the manifest, and the compiler checks both copies against the real parser
// either way.

// link builds a one-link set and returns that link's instance, so a test reads
// as the manifest a user would write.
func link(t *testing.T, body string) instances.Instance {
	t.Helper()
	m, err := manifest.Parse([]byte("[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\nworkspace = 'ws_abc123'\n" + body))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	set := instances.Build(m)
	all := set.All()
	if len(all) != 1 {
		t.Fatalf("expected one link, got %v", set.Names())
	}
	return all[0]
}

// header runs a credential source and returns the Authorization header it would
// produce, which is what actually matters about it.
func header(t *testing.T, src airflowapi.CredentialSource) string {
	t.Helper()
	if src == nil {
		return ""
	}
	scheme, value, err := src(context.Background())
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if scheme == "" {
		return ""
	}
	return scheme + " " + value
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
