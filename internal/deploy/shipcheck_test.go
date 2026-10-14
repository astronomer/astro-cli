package deploy

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

func linkedManifest() *manifest.Manifest {
	return &manifest.Manifest{Astro: manifest.Astro{
		Deployments: map[string]manifest.Link{
			"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		},
	}}
}

// A gitignored file the image would carry that looks like a credential stops
// an image deploy before anything is asked or built; a dags-only deploy builds
// no image and is not held to it.
func TestRun_RefusesGitignoredCredentials(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.pem\nbuild/\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prod.pem"), []byte("k"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "build"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build", "out.txt"), []byte("o"), 0o600))

	d := &fakeDeployer{}
	_, err = Run(Request{ProjectDir: dir, LinkName: "prod", Manifest: linkedManifest()}, d)
	require.ErrorContains(t, err, "prod.pem look like credentials")
	assert.Zero(t, d.imgDeploys)

	_, err = Run(Request{ProjectDir: dir, LinkName: "prod", DagsOnly: true, Manifest: linkedManifest()}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.deploys)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("*.pem\n"), 0o600))
	var warned []string
	_, err = Run(Request{ProjectDir: dir, LinkName: "prod", Manifest: linkedManifest(), Warn: func(w string) { warned = append(warned, w) }}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.imgDeploys)
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "build/out.txt")
}

// The transport's warnings reach Request.Warn, and an image deploy whose DAGs
// went inside the image says so on its result.
func TestRun_CarriesWarnAndDagsInImage(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{ImageTag: "deploy-2026", DagsInImage: true}}
	warn := func(string) {}
	res, err := Run(Request{ProjectDir: t.TempDir(), LinkName: "prod", Manifest: linkedManifest(), Warn: warn}, d)
	require.NoError(t, err)
	assert.NotNil(t, d.imgInput.Warn)
	assert.NotNil(t, d.imgInput.Survey, "the survey the checks made is handed on, not made again")
	assert.Equal(t, "image-and-dag", res.Type)
	assert.True(t, res.DagsInImage)
	assert.Empty(t, res.DagTarballVersion)
}

// The engine is resolved only by a deploy that builds, and the resolver is
// handed on to the transport rather than resolved again.
func TestRun_ResolvesTheEngineOnlyToBuild(t *testing.T) {
	calls := 0
	engine := func() (string, []string, error) {
		calls++
		return "", nil, errors.New("no engine")
	}
	d := &fakeDeployer{}
	_, err := Run(Request{ProjectDir: t.TempDir(), LinkName: "prod", Manifest: linkedManifest(), DagsOnly: true, Engine: engine}, d)
	require.NoError(t, err)
	_, err = Run(Request{ProjectDir: t.TempDir(), LinkName: "prod", Manifest: linkedManifest(), ImageName: "img:1", Engine: engine}, d)
	require.NoError(t, err)
	assert.Zero(t, calls, "a dags-only or prebuilt deploy builds nothing")

	_, err = Run(Request{ProjectDir: t.TempDir(), LinkName: "prod", Manifest: linkedManifest(), Engine: engine}, d)
	require.NoError(t, err, "an engine that cannot be resolved is the transport's to report")
	assert.Equal(t, 1, calls)
	assert.NotNil(t, d.imgInput.Engine)
}
