package deploy

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// imageSourceProject writes a linked project, with a Dockerfile when from is
// set, and returns its root and loaded manifest.
func imageSourceProject(t *testing.T, astro, from string) (string, *manifest.Manifest) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"+astro+
		"\n[tool.astro.deployments.prod]\nworkspace = 'ws-prod'\ndeployment = 'dep-prod'\n")
	if from != "" {
		writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM "+from+"\n")
	}
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	return dir, m
}

// An image deploy refuses a Dockerfile whose FROM names another Airflow, before
// it asks where to ship or builds anything. A dags-only deploy and a prebuilt
// image build nothing from the Dockerfile, and go ahead.
func TestRunRefusesADockerfileOfAnotherAirflow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		req     func(Request) Request
		refused bool
	}{
		{name: "default", req: func(r Request) Request { return r }, refused: true},
		{name: "--image", req: func(r Request) Request { r.Image = true; return r }, refused: true},
		{name: "--dags", req: func(r Request) Request { r.DagsOnly = true; return r }},
		{name: "--image-name", req: func(r Request) Request { r.ImageName = "prebuilt:1"; return r }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, m := imageSourceProject(t, "dockerfile = 'Dockerfile'\n", "astrocrpublic.azurecr.io/runtime:3.3-8")
			d := &fakeDeployer{}
			_, err := Run(tc.req(Request{ProjectDir: dir, Manifest: m, LinkName: "prod"}), d)
			var ve *manifest.ValidationError
			if !tc.refused {
				require.NoError(t, err)
				return
			}
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, manifest.CodeDockerfileAirflowMismatch, ve.Problems[0].Code)
			assert.Zero(t, d.imgDeploys+d.deploys+d.confirms+d.resolves, "nothing was asked or shipped")
		})
	}
}

func TestRunChecksTheRuntimeBuild(t *testing.T) {
	dir, m := imageSourceProject(t, "runtime = '3.1-12'\n", "")
	var (
		calls    [][2]string
		warnings []string
	)
	req := Request{
		ProjectDir: dir, Manifest: m, LinkName: "prod",
		CheckRuntime: func(runtime, pin string) ([]runtimeversions.Finding, error) {
			calls = append(calls, [2]string{runtime, pin})
			return []runtimeversions.Finding{{Kind: runtimeversions.FindingYanked, Message: "yanked"}}, nil
		},
		Warn: func(s string) { warnings = append(warnings, s) },
	}
	d := &fakeDeployer{}
	_, err := Run(req, d)
	require.NoError(t, err)
	assert.Equal(t, [][2]string{{"3.1-12", "3.1"}}, calls)
	assert.Equal(t, []string{"pyproject.toml: tool.astro.runtime: yanked"}, warnings)
	assert.Equal(t, "3.1-12", d.imgInput.Runtime, "the build reaches the transport")

	// A blocking finding stops the deploy before anything is shipped.
	blocking := errors.New("another series")
	req.CheckRuntime = func(string, string) ([]runtimeversions.Finding, error) { return nil, blocking }
	d = &fakeDeployer{}
	_, err = Run(req, d)
	require.ErrorIs(t, err, blocking)
	assert.Zero(t, d.imgDeploys)

	// A dags-only deploy builds no image, and does not ask.
	calls = nil
	req.CheckRuntime = func(runtime, pin string) ([]runtimeversions.Finding, error) {
		calls = append(calls, [2]string{runtime, pin})
		return nil, nil
	}
	req.DagsOnly = true
	_, err = Run(req, &fakeDeployer{})
	require.NoError(t, err)
	assert.Empty(t, calls)
}

// An image deploy warns about a secret the declared Dockerfile mounts that no
// --build-secret supplies, before it builds, and still ships.
func TestRunWarnsAboutAnUnsuppliedSecretMount(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "machine example.com")
	dir, m := imageSourceProject(t, "dockerfile = 'Dockerfile'\n", "")
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN --mount=type=secret,id=netrc pip install private\n")
	var warnings []string
	req := Request{ProjectDir: dir, Manifest: m, LinkName: "prod", Warn: func(s string) { warnings = append(warnings, s) }}

	d := &fakeDeployer{}
	_, err := Run(req, d)
	require.NoError(t, err)
	assert.Equal(t, []string{`Dockerfile mounts build secret "netrc" (line 2) but none was given; pass --build-secret id=netrc,env=<VAR> or set BUILD_SECRET_INPUT`}, warnings)
	assert.Equal(t, 1, d.imgDeploys)

	warnings = nil
	req.BuildSecrets = []string{"id=netrc,env=NETRC_CONTENT"}
	_, err = Run(req, &fakeDeployer{})
	require.NoError(t, err)
	assert.Empty(t, warnings)

	// Neither of these builds the Dockerfile, so neither warns.
	req.BuildSecrets = nil
	for _, r := range []Request{{DagsOnly: true}, {ImageName: "prebuilt:1"}} {
		r.ProjectDir, r.Manifest, r.LinkName, r.Warn = req.ProjectDir, req.Manifest, req.LinkName, req.Warn
		_, err = Run(r, &fakeDeployer{})
		require.NoError(t, err)
	}
	assert.Empty(t, warnings)
}

// A failed image build names the unsupplied secret again in the deploy's
// error, since the build output has pushed the warning out of sight.
func TestRunFailedBuildNamesTheUnsuppliedSecretMount(t *testing.T) {
	dir, m := imageSourceProject(t, "dockerfile = 'Dockerfile'\n", "")
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN --mount=type=secret,id=netrc pip install private\n")
	req := Request{ProjectDir: dir, Manifest: m, LinkName: "prod"}
	buildErr := fmt.Errorf("%w: exit status 1", imagebuild.ErrDockerfileBuild)

	_, err := Run(req, &fakeDeployer{imgErr: buildErr})
	require.ErrorIs(t, err, imagebuild.ErrDockerfileBuild)
	assert.Contains(t, err.Error(), `exit status 1 — Dockerfile mounts build secret "netrc", which was not given; pass --build-secret id=netrc,env=<VAR>`)

	other := errors.New("pushing the image: unauthorized")
	_, err = Run(req, &fakeDeployer{imgErr: other})
	assert.Equal(t, other, err, "only a failed Dockerfile build gets the hint")
}

// An image deploy whose build secret names an unset variable stops before it
// builds or ships anything.
func TestRunRefusesABuildSecretWhoseVariableIsUnset(t *testing.T) {
	t.Setenv("NETRC_CONTENT", "")
	dir, m := imageSourceProject(t, "dockerfile = 'Dockerfile'\n", "")
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN --mount=type=secret,id=netrc pip install private\n")
	req := Request{ProjectDir: dir, Manifest: m, LinkName: "prod", BuildSecrets: []string{"id=netrc,env=NETRC_CONTENT"}}

	d := &fakeDeployer{}
	_, err := Run(req, d)
	require.ErrorContains(t, err, "reads the environment variable NETRC_CONTENT")
	assert.Zero(t, d.imgDeploys)

	req.DagsOnly = true
	_, err = Run(req, &fakeDeployer{})
	require.NoError(t, err, "a dags-only deploy builds nothing")
}
