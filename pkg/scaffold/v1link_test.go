package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

const (
	v1DeploymentID = "cm1ordersdeployment000001"
	v1WorkspaceID  = "cm1ordersworkspace0000001"
)

// convertedManifest converts dir and loads the manifest it wrote.
func convertedManifest(t *testing.T, dir string) (*Result, *manifest.Manifest) {
	t.Helper()
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	return res, m
}

func v1ProjectWithConfig(t *testing.T, config string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	files := map[string]string{v1ConfigRelPath: config, "Dockerfile": pinOnlyDockerfile}
	for k, v := range extra {
		files[k] = v
	}
	writeAll(t, dir, files)
	return dir
}

// A saved Astro Deployment with its workspace becomes a link, marked default,
// in both arms, and is reported as done rather than as left to do.
func TestASavedDeployTargetBecomesALink(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + v1DeploymentID + "\n  workspace: " + v1WorkspaceID + "\n"
	for name, extra := range map[string]map[string]string{
		"greenfield": nil,
		"adopted":    {manifest.Marker: "[tool.ruff]\nline-length = 120\n"},
	} {
		t.Run(name, func(t *testing.T) {
			res, m := convertedManifest(t, v1ProjectWithConfig(t, config, extra))
			link, ok := m.Astro.Deployments[V1LinkName]
			require.True(t, ok, "links: %v", m.Astro.Deployments)
			assert.Equal(t, v1DeploymentID, link.Deployment)
			assert.Equal(t, v1WorkspaceID, link.Workspace)
			assert.True(t, link.Default, "the one saved target is the default")
			assert.Equal(t, manifest.KindAstro, link.Kind())
			for _, n := range res.Notes {
				assert.NotContains(t, n, "saved deploy target", "a linked target is not left to do")
			}
			assert.Contains(t, strings.Join(res.Advisories, "\n"), "[tool.astro.deployments."+V1LinkName+"]")
		})
	}
}

// The link is SaveLink's: the same bytes `astro link add` writes into the
// same manifest.
func TestTheConversionLinkIsWhatSaveLinkWrites(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + v1DeploymentID + "\n  workspace: " + v1WorkspaceID + "\n"
	dir := v1ProjectWithConfig(t, config, nil)
	_, err := Run(dir, Options{})
	require.NoError(t, err)
	converted, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)

	plain := v1ProjectWithConfig(t, "project:\n  name: orders\n", nil)
	_, err = Run(plain, Options{})
	require.NoError(t, err)
	require.NoError(t, SaveLink(plain, nil, Link{Name: V1LinkName, Kind: manifest.KindAstro, Deployment: v1DeploymentID, Workspace: v1WorkspaceID}))
	require.NoError(t, SetDefaultLink(plain, nil, V1LinkName))
	linked, err := os.ReadFile(filepath.Join(plain, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, string(linked), string(converted))
}

// A link needs both ids in their Astro shape. Anything less stays a note, and
// no link is written.
func TestASavedDeployTargetThatCannotBeLinkedStaysANote(t *testing.T) {
	for name, config := range map[string]string{
		// cmd/apc/deploy.go saves a Software release name under the same key.
		"a Software release name": "project:\n  name: orders\n  deployment: celestial-orbit-1234\n  workspace: " + v1WorkspaceID + "\n",
		// A link must name a workspace, and the conversion cannot look one up.
		"no workspace":       "project:\n  name: orders\n  deployment: " + v1DeploymentID + "\n",
		"a workspace not id": "project:\n  name: orders\n  deployment: " + v1DeploymentID + "\n  workspace: my workspace\n",
	} {
		t.Run(name, func(t *testing.T) {
			res, m := convertedManifest(t, v1ProjectWithConfig(t, config, nil))
			assert.Empty(t, m.Astro.Deployments)
			assert.Contains(t, strings.Join(res.Notes, "\n"), "saved deploy target")
			assert.NotContains(t, strings.Join(res.Advisories, "\n"), "[tool.astro.deployments.")
		})
	}
}
