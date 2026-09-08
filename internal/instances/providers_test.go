package instances

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// refusingLocator fails loudly, so a test can prove the refusal happened before
// any coordinate was looked up.
type refusingLocator struct{ asked *bool }

func (l refusingLocator) BaseURL(context.Context, Instance) (string, error) {
	*l.asked = true
	return "", errors.New("the locator was asked")
}

// A method this build cannot perform is refused before any URL is looked up.
//
// This is the finding that made an earlier draft's refusal unreachable for
// every real Composer deployment. A composer link carries coordinates and no
// url, so resolving it means asking the Locator — a network call — and the
// credential registry sat two layers below that. What came back was whatever
// the lookup hit, never the message the whole seam exists to produce.
func TestAnUnsupportedMethodIsRefusedBeforeAnyLookup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		link   string
		method manifest.AuthMethod
	}{
		{
			name:   "composer, which resolves its url through the locator",
			link:   "\n[tool.astro.deployments.gcp-prod]\ntarget = 'composer'\nenvironment = 'orders-prod'\n",
			method: manifest.AuthGoogle,
		},
		{
			name:   "mwaa, which has no url at all",
			link:   "\n[tool.astro.deployments.airflow-team]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n",
			method: manifest.AuthAWS,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			i := link(t, tc.link)
			// No Providers: the build that talks to neither platform.
			_, err := i.Transport(context.Background(), Deps{Locator: refusingLocator{asked: &asked}})
			if err == nil {
				t.Fatal("resolved a method this build cannot perform")
			}
			if asked {
				t.Error("asked the locator before refusing")
			}
			for _, want := range []string{i.Name, string(tc.method), "does not carry", "astro use"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to mention %q", err, want)
				}
			}
		})
	}
}

// The refusal names the method, and the name is not a substring of the
// deployment's own.
//
// An earlier version of this test asserted "aws" against a deployment called
// "aws-prod", so the method half could never fail — and the instance it used
// carried no environment, so the code path it was guarding errored anyway for
// an unrelated reason. Both assertions passed against the wrong error.
func TestTheRefusalNamesTheMethodNotJustTheDeployment(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.orders-pipeline]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n")

	// With a provider, this exact instance resolves — so the refusal below is
	// the only thing the assertion can be reading. A stand-in rather than the
	// real AWS door: what this package owes a consumer is the dispatch, and the
	// door lives in a package that imports this one.
	carrying := Providers{manifest.AuthAWS: {
		Transport: func(context.Context, Instance, Deps) (airflowapi.Transport, error) {
			return stubTransport{}, nil
		},
	}}
	if _, err := i.Transport(context.Background(), Deps{Providers: carrying}); err != nil {
		t.Fatalf("the carrying build could not resolve it: %v", err)
	}

	_, err := i.Transport(context.Background(), Deps{})
	if err == nil {
		t.Fatal("resolved without a provider")
	}
	if !strings.Contains(err.Error(), string(manifest.AuthAWS)) {
		t.Errorf("err = %v, want it to name the method", err)
	}
}

// Every method that needs no provider still works in a build that carries none.
//
// All six, named individually: this is the list a consumer keeps by leaving the
// cloud providers out, so a change that quietly routed one of them through a
// provider should fail here rather than in that consumer.
func TestTheSixCheapMethodsNeedNoProvider(t *testing.T) {
	const url = "https://airflow.example.com"
	for _, tc := range []struct {
		method manifest.AuthMethod
		auth   string
		env    map[string]string
		deps   Deps
	}{
		{method: manifest.AuthNone, auth: "{ method = 'none' }"},
		{
			method: manifest.AuthAstro,
			auth:   "{ method = 'astro' }",
			deps:   Deps{Session: func(context.Context) (string, error) { return "astro-token", nil }},
		},
		{
			method: manifest.AuthToken,
			auth:   "{ method = 'token', token-env = 'AF_TOKEN' }",
			env:    map[string]string{"AF_TOKEN": "t"},
		},
		{
			method: manifest.AuthBasic,
			auth:   "{ method = 'basic', username-env = 'AF_USER', password-env = 'AF_PASSWORD' }",
			env:    map[string]string{"AF_USER": "u", "AF_PASSWORD": "p"},
		},
		{
			method: manifest.AuthAirflowToken,
			auth:   "{ method = 'airflow-token', username-env = 'AF_USER', password-env = 'AF_PASSWORD' }",
			env:    map[string]string{"AF_USER": "u", "AF_PASSWORD": "p"},
		},
		{
			method: manifest.AuthExec,
			auth:   "{ method = 'exec', command = ['true'] }",
		},
	} {
		t.Run(string(tc.method), func(t *testing.T) {
			i := link(t, "\n[tool.astro.deployments.oss]\nurl = '"+url+"'\nauth = "+tc.auth+"\n")
			d := tc.deps
			d.LookupEnv = env(tc.env)
			// Deliberately no Providers.
			if _, _, err := credentials(context.Background(), i, url, d); err != nil {
				t.Errorf("%s needed a provider: %v", tc.method, err)
			}
		})
	}
}

// A provider that hands back no credential is refused, not passed on.
//
// "Nothing to add" is a natural thing for a provider to return when a machine
// has no credential to offer, and passing it through sends the request with no
// Authorization header — which arrives as a bare 401 from Airflow and reads as
// a credential the user got wrong. The missing-provider case was guarded from
// the start; this one is a line later and was not.
func TestAProviderThatOffersNoCredentialIsRefused(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.gcp-prod]\nurl = 'https://composer.example.com'\nauth = { method = 'google' }\n")
	empty := Providers{manifest.AuthGoogle: {
		Credentials: func(context.Context, Instance, string, Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
			return nil, nil, nil
		},
	}}
	_, _, err := credentials(context.Background(), i, i.URL, Deps{Providers: empty, LookupEnv: env(nil)})
	if err == nil {
		t.Fatal("accepted a provider that offered no credential")
	}
	if !strings.Contains(err.Error(), "no credential") {
		t.Errorf("err = %v, want it to say the provider returned none", err)
	}
}

// Providers are per-Deps, so two in one process do not share state.
//
// The shape this replaced was two package-level maps: a process global that
// resolution read, which made a second provider set impossible and any
// registration outside init() a data race.
func TestTwoBuildsCanDisagreeInOneProcess(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.gcp-prod]\nurl = 'https://composer.example.com'\nauth = { method = 'google' }\n")

	carrying := Providers{manifest.AuthGoogle: {
		Credentials: func(context.Context, Instance, string, Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
			return func(context.Context) (string, string, error) { return "Bearer", "ya29.token", nil }, nil, nil
		},
	}}
	if _, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: env(nil), Providers: carrying}); err != nil {
		t.Fatalf("the carrying build failed: %v", err)
	}
	if _, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: env(nil)}); err == nil {
		t.Error("the build carrying nothing resolved it anyway")
	}
}

// stubTransport stands in for a door, so a dispatch test needs no vendor SDK.
type stubTransport struct{}

func (stubTransport) Do(context.Context, airflowapi.Request) (airflowapi.Response, error) {
	return airflowapi.Response{}, nil
}
