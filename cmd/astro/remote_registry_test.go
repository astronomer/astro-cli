package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

func registryManifest(t *testing.T, astro string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse([]byte("[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws'\n" + astro))
	require.NoError(t, err)
	return m
}

// --registry, else the named link's client-registry, else the project's.
func TestResolveClientTarget(t *testing.T) {
	m := registryManifest(t, `
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
`)
	for _, tc := range []struct {
		name, registry, deployment string
		want                       clientTarget
	}{
		{"the project's", "", "", clientTarget{registry: "quay.io/acme/client", source: projectClientRegistry}},
		{
			"a link without its own takes the project's, and its Deployment", "", "dev",
			clientTarget{registry: "quay.io/acme/client", source: projectClientRegistry, deploymentID: "dep-dev"},
		},
		{
			"a link's own", "", "prod",
			clientTarget{registry: "123.dkr.ecr.us-east-1.amazonaws.com/acme-prod", source: "pyproject.toml [tool.astro.deployments.prod] client-registry", deploymentID: "dep-prod"},
		},
		{
			"--registry wins over both", "other.example.com/x", "prod",
			clientTarget{registry: "other.example.com/x", source: "--registry", deploymentID: "dep-prod"},
		},
		{
			"a linked Deployment named by its id takes its link's", "", "dep-prod",
			clientTarget{registry: "123.dkr.ecr.us-east-1.amazonaws.com/acme-prod", source: "pyproject.toml [tool.astro.deployments.prod] client-registry", deploymentID: "dep-prod"},
		},
		{
			"a Deployment id that is no link is passed as it is", "", "clnotalink",
			clientTarget{registry: "quay.io/acme/client", source: projectClientRegistry, deploymentID: "clnotalink"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveClientTarget(m, tc.registry, tc.deployment)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := resolveClientTarget(m, "", "aws")
	assert.ErrorContains(t, err, "only an Astro Deployment runs remote agents")
}

// With no --deployment, the project's default link gives its registry, but
// not its Deployment: a run that named none is not checked against one.
func TestResolveClientTargetTakesTheDefaultLinksRegistry(t *testing.T) {
	m := registryManifest(t, `
[tool.astro.deployments.prod]
deployment = 'dep-prod'
default = true
client-registry = 'quay.io/acme/prod'

[tool.astro.deployments.dev]
deployment = 'dep-dev'
`)
	got, err := resolveClientTarget(m, "", "")
	require.NoError(t, err)
	assert.Equal(t, clientTarget{registry: "quay.io/acme/prod", source: "pyproject.toml [tool.astro.deployments.prod] client-registry"}, got)
}

func TestResolveClientTargetWithNone(t *testing.T) {
	_, err := resolveClientTarget(nil, "", "")
	assert.ErrorContains(t, err, "pass --registry <registry>/<repository>")

	got, err := resolveClientTarget(nil, "quay.io/acme/client", "dep")
	require.NoError(t, err)
	assert.Equal(t, clientTarget{registry: "quay.io/acme/client", source: "--registry", deploymentID: "dep"}, got)

	_, err = resolveClientTarget(registryManifest(t, ""), "", "")
	assert.ErrorContains(t, err, "set client-registry under [tool.astro.deploy] in pyproject.toml")
}
