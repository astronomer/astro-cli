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

// --registry, else the named link's agent-registry, else the project's.
func TestResolveAgentTarget(t *testing.T) {
	m := registryManifest(t, `
[tool.astro.deploy]
agent-registry = 'quay.io/acme/client'

[tool.astro.deployments.dev]
deployment = 'dep-dev'

[tool.astro.deployments.prod]
deployment = 'dep-prod'
agent-registry = '123.dkr.ecr.us-east-1.amazonaws.com/acme-prod'

[tool.astro.deployments.aws]
target = 'mwaa'
environment = 'orders'
`)
	for _, tc := range []struct {
		name, registry, deployment string
		want                       agentTarget
	}{
		{"the project's", "", "", agentTarget{registry: "quay.io/acme/client", source: projectAgentRegistry}},
		{
			"a link without its own takes the project's, and its Deployment", "", "dev",
			agentTarget{registry: "quay.io/acme/client", source: projectAgentRegistry, deploymentID: "dep-dev"},
		},
		{
			"a link's own", "", "prod",
			agentTarget{registry: "123.dkr.ecr.us-east-1.amazonaws.com/acme-prod", source: "pyproject.toml [tool.astro.deployments.prod] agent-registry", deploymentID: "dep-prod"},
		},
		{
			"--registry wins over both", "other.example.com/x", "prod",
			agentTarget{registry: "other.example.com/x", source: "--registry", deploymentID: "dep-prod"},
		},
		{
			"a linked Deployment named by its id takes its link's", "", "dep-prod",
			agentTarget{registry: "123.dkr.ecr.us-east-1.amazonaws.com/acme-prod", source: "pyproject.toml [tool.astro.deployments.prod] agent-registry", deploymentID: "dep-prod"},
		},
		{
			"a Deployment id that is no link is passed as it is", "", "clnotalink",
			agentTarget{registry: "quay.io/acme/client", source: projectAgentRegistry, deploymentID: "clnotalink"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAgentTarget(m, tc.registry, tc.deployment)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := resolveAgentTarget(m, "", "aws")
	assert.ErrorContains(t, err, "only an Astro Deployment runs Remote Execution agents")
}

// With no --deployment, the project's default link gives its registry, but
// not its Deployment: a run that named none is not checked against one.
func TestResolveAgentTargetTakesTheDefaultLinksRegistry(t *testing.T) {
	m := registryManifest(t, `
[tool.astro.deployments.prod]
deployment = 'dep-prod'
default = true
agent-registry = 'quay.io/acme/prod'

[tool.astro.deployments.dev]
deployment = 'dep-dev'
`)
	got, err := resolveAgentTarget(m, "", "")
	require.NoError(t, err)
	assert.Equal(t, agentTarget{registry: "quay.io/acme/prod", source: "pyproject.toml [tool.astro.deployments.prod] agent-registry"}, got)
}

func TestResolveAgentTargetWithNone(t *testing.T) {
	_, err := resolveAgentTarget(nil, "", "")
	assert.ErrorContains(t, err, "pass --registry <registry>/<repository>")

	got, err := resolveAgentTarget(nil, "quay.io/acme/client", "dep")
	require.NoError(t, err)
	assert.Equal(t, agentTarget{registry: "quay.io/acme/client", source: "--registry", deploymentID: "dep"}, got)

	_, err = resolveAgentTarget(registryManifest(t, ""), "", "")
	assert.ErrorContains(t, err, "set agent-registry under [tool.astro.deploy] in pyproject.toml")
}
