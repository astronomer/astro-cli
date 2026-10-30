package pack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The image is built from dependencies and OS packages alone, so a project
// that declares its environment is told the image carries none of it, and
// what the Deployment has to supply.
func TestAstroBuildListsTheDeclaredEnvironmentForTheDeployment(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Astro.Env = map[string]any{
		"LOG_LEVEL": "info",
		"API_KEY":   map[string]any{},
		"connections": map[string]any{
			"warehouse": map[string]any{"source": "workspace"},
			"archive":   map[string]any{"optional": true},
			"main":      map[string]any{},
		},
	}
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	// The required ones first, then by kind and name.
	assert.Equal(t, []DeploymentValue{
		{Kind: "connection", Name: "main", Status: "required"},
		{Kind: "env var", Name: "API_KEY", Status: "required"},
		{Kind: "connection", Name: "archive", Status: "optional"},
		{Kind: "connection", Name: "warehouse", Status: "from workspace"},
		{Kind: "env var", Name: "LOG_LEVEL", Status: "has a default"},
	}, res.DeploymentEnv)
	// A list for the Deployment, not a warning: the image never carries them.
	for _, w := range res.Warnings {
		assert.NotContains(t, w, "[tool.astro.env]")
	}
}

func TestAstroBuildSaysNothingAboutAnEnvironmentItDoesNotDeclare(t *testing.T) {
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.DeploymentEnv)
}

// A declaration the parser refuses stops the package, before any build.
func TestAstroBuildRefusesADeclarationThatDoesNotParse(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Astro.Env = map[string]any{"BAD": 5}
	builder := &fakeBuilder{}
	_, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BAD")
	assert.Empty(t, builder.gotReq.Tag, "nothing was built")
}
