package manifest

import (
	"testing"
)

const deployBase = "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws-abc'\n"

// Without [tool.astro.deploy], a deploy records its commit and tags by name:
// the defaults Astro CLI 1.x shipped.
func TestDeployDefaults(t *testing.T) {
	m, err := Parse([]byte(deployBase))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Deploy{GitMetadata: true}); m.Astro.Deploy != want {
		t.Errorf("got %+v, want %+v", m.Astro.Deploy, want)
	}
}

func TestDeploySettings(t *testing.T) {
	m, err := Parse([]byte(deployBase + "\n[tool.astro.deploy]\nclient-registry = 'quay.io/acme/client'\ncosmos-boost = true\ngit-metadata = false\nsha-as-tag = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Deploy{ClientRegistry: "quay.io/acme/client", CosmosBoost: true, ShaAsTag: true}); m.Astro.Deploy != want {
		t.Errorf("got %+v, want %+v", m.Astro.Deploy, want)
	}
}

// A link's client registry is its own, else the project's; only astro links
// have one.
func TestLinkClientRegistry(t *testing.T) {
	m, err := Parse([]byte(deployBase + `
[tool.astro.deploy]
client-registry = 'quay.io/acme/client'

[tool.astro.deployments.dev]
deployment = 'dep-dev'

[tool.astro.deployments.prod]
deployment = 'dep-prod'
client-registry = '123.dkr.ecr.us-east-1.amazonaws.com/acme-prod'

[tool.astro.deployments.aws]
target = 'mwaa'
environment = 'orders'
`))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"dev":  "quay.io/acme/client",
		"prod": "123.dkr.ecr.us-east-1.amazonaws.com/acme-prod",
		"aws":  "",
	} {
		if got := m.Astro.Deployments[name].ClientRegistry; got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

// Each value has its type, and an empty registry is a mistake, not "unset".
func TestDeploySettingsOfTheWrongShape(t *testing.T) {
	for name, tc := range map[string]struct {
		table   string
		wantKey string
		code    ProblemCode
	}{
		"a registry that is not a string": {"client-registry = 1\n", "tool.astro.deploy.client-registry", CodeExpectedString},
		"an empty registry":               {"client-registry = ''\n", "tool.astro.deploy.client-registry", CodeEmptyString},
		"git-metadata that is not a bool": {"git-metadata = 'no'\n", "tool.astro.deploy.git-metadata", CodeExpectedBool},
		"sha-as-tag that is not a bool":   {"sha-as-tag = 1\n", "tool.astro.deploy.sha-as-tag", CodeExpectedBool},
		"cosmos-boost that is not a bool": {"cosmos-boost = 'yes'\n", "tool.astro.deploy.cosmos-boost", CodeExpectedBool},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(deployBase + "\n[tool.astro.deploy]\n" + tc.table))
			ve := validationError(t, err)
			if len(ve.Problems) != 1 || ve.Problems[0].Code != tc.code || ve.Problems[0].Key != tc.wantKey {
				t.Errorf("want %s on %s, got %+v", tc.code, tc.wantKey, ve.Problems)
			}
		})
	}
}

// An unknown key in the table is a warning, as anywhere in [tool.astro].
func TestAnUnknownDeployKeyWarns(t *testing.T) {
	m, err := Parse([]byte(deployBase + "\n[tool.astro.deploy]\ncosmos_boost = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 1 || m.Warnings[0].Key != "tool.astro.deploy.cosmos_boost" {
		t.Errorf("got %+v", m.Warnings)
	}
}
