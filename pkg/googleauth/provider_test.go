package googleauth

import (
	"context"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The provider carries the chain it was given, resolved through the real
// dispatch.
//
// This is the test the first version of this package did not have, and its
// absence was invisible: changing Provider to build its credentials from a
// fresh Options{} instead of the one it was handed passed the entire suite,
// because everything else called googleCredentials directly. A Composer command
// would then abandon the token the environment lookup already resolved and walk
// the machine's live ADC chain instead — the two-chains failure the wiring in
// cmd/local exists to prevent, and which no test could see.
func TestTheProviderCarriesTheChainItWasGiven(t *testing.T) {
	asked := 0
	p := Provider(Options{
		Token: func(context.Context) (string, error) {
			asked++
			return "ya29.from-the-lookup", nil
		},
	})

	i := link(t, "\n[tool.astro.deployments.gcp-prod]\nurl = 'https://composer.example.com'\nauth = { method = 'google' }\n")
	door, err := i.HTTPDoorFor(context.Background(), instances.Deps{
		Providers: instances.Providers{manifest.AuthGoogle: p},
	})
	if err != nil {
		t.Fatalf("door: %v", err)
	}
	if door.Authorization != "Bearer ya29.from-the-lookup" {
		t.Errorf("authorization = %q, want the token the options carried", door.Authorization)
	}
	if asked != 1 {
		t.Errorf("the options' chain was asked %d times, want once", asked)
	}
}

// A machine with no chain is refused, and the refusal names the command that
// fixes it rather than surfacing as an empty credential.
func TestAMissingChainReachesTheCaller(t *testing.T) {
	p := Provider(Options{
		Token: func(context.Context) (string, error) { return "", ErrNoCredentials },
	})
	i := link(t, "\n[tool.astro.deployments.gcp-prod]\nurl = 'https://composer.example.com'\nauth = { method = 'google' }\n")
	_, err := i.HTTPDoorFor(context.Background(), instances.Deps{
		Providers: instances.Providers{manifest.AuthGoogle: p},
	})
	if err == nil {
		t.Fatal("a missing chain produced a door")
	}
	if !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Errorf("err = %v, want the ADC message", err)
	}
}

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
