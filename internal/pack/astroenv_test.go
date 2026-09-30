package pack

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The image is built from dependencies and OS packages alone, so a project
// that declares its environment is told the image carries none of it, and
// what the Deployment has to supply.
func TestAstroBuildWarnsThatTheImageCarriesNoDeclaredEnvironment(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Astro.Env = map[string]any{
		"LOG_LEVEL": "info",
		"API_KEY":   map[string]any{},
		"connections": map[string]any{
			"warehouse": map[string]any{"source": "workspace"},
		},
	}
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	var got string
	for _, w := range res.Warnings {
		if strings.Contains(w, "[tool.astro.env]") {
			got = w
		}
	}
	require.NotEmpty(t, got, "warnings: %v", res.Warnings)
	for _, want := range []string{
		"none of the 3 value(s)",
		"on the Deployment",
		"env var API_KEY (required)",
		"env var LOG_LEVEL (has a default)",
		"connection warehouse (from workspace)",
	} {
		assert.Contains(t, got, want)
	}
}

func TestAstroBuildSaysNothingAboutAnEnvironmentItDoesNotDeclare(t *testing.T) {
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	for _, w := range res.Warnings {
		assert.NotContains(t, w, "[tool.astro.env]")
	}
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
