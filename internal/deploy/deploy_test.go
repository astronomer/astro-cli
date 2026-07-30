package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

const v2Manifest = `[project]
name = "demo"

[tool.astro]
airflow = "3.1"
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestIsV2Project(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pyproject.toml"), v2Manifest)
		assert.True(t, IsV2Project(dir))
	})

	t.Run("valid manifest beside a Dockerfile is still v2", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pyproject.toml"), v2Manifest)
		writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM x\n")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
		assert.True(t, IsV2Project(dir))
	})

	t.Run("missing pyproject", func(t *testing.T) {
		assert.False(t, IsV2Project(t.TempDir()))
	})

	t.Run("v1 layout is not v2", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM quay.io/astronomer/astro-runtime:1\n")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
		assert.False(t, IsV2Project(dir))
	})

	t.Run("pyproject without tool.astro", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = \"demo\"\n")
		assert.False(t, IsV2Project(dir))
	})

	t.Run("tool.astro present but invalid still routes to v2", func(t *testing.T) {
		dir := t.TempDir()
		// [tool.astro] with no airflow version fails validation, but it is still
		// a v2 project whose error should surface on the v2 path.
		writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = \"demo\"\n\n[tool.astro]\n")
		assert.True(t, IsV2Project(dir))
	})

	t.Run("unparseable pyproject routes to v2", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pyproject.toml"), "this is not : valid = toml [[[\n")
		assert.True(t, IsV2Project(dir))
	})
}

func manifestWith(links map[string]manifest.Link) *manifest.Manifest {
	return &manifest.Manifest{Astro: manifest.Astro{Deployments: links}}
}

func TestResolveSelection(t *testing.T) {
	oneLink := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
	}
	twoLinks := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		"dev":  {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
	}

	t.Run("--deployment overrides the manifest", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest:         manifestWith(oneLink),
			DeploymentID:     "dep-flag",
			ContextWorkspace: "ws-ctx",
		})
		require.NoError(t, err)
		assert.Equal(t, "dep-flag", sel.deploymentID)
		assert.Equal(t, "ws-ctx", sel.workspaceID)
	})

	t.Run("--deployment takes --workspace over context", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			DeploymentID:     "dep-flag",
			WorkspaceID:      "ws-flag",
			ContextWorkspace: "ws-ctx",
		})
		require.NoError(t, err)
		assert.Equal(t, "ws-flag", sel.workspaceID)
	})

	t.Run("named link", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest: manifestWith(twoLinks),
			LinkName: "dev",
		})
		require.NoError(t, err)
		assert.Equal(t, "dep-dev", sel.deploymentID)
		assert.Equal(t, "ws-dev", sel.workspaceID)
		assert.Equal(t, "dev", sel.linkName)
	})

	t.Run("named link not found", func(t *testing.T) {
		_, err := resolveSelection(Request{
			Manifest: manifestWith(twoLinks),
			LinkName: "staging",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "staging")
		assert.Contains(t, err.Error(), "dev, prod")
	})

	t.Run("--workspace overrides a link's workspace", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest:    manifestWith(oneLink),
			LinkName:    "prod",
			WorkspaceID: "ws-flag",
		})
		require.NoError(t, err)
		assert.Equal(t, "ws-flag", sel.workspaceID)
	})

	t.Run("default link when exactly one", func(t *testing.T) {
		sel, err := resolveSelection(Request{Manifest: manifestWith(oneLink)})
		require.NoError(t, err)
		assert.Equal(t, "dep-prod", sel.deploymentID)
		assert.Equal(t, "prod", sel.linkName)
	})

	markedDefault := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		"dev":  {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev", Default: true},
	}

	t.Run("the link marked default = true is chosen", func(t *testing.T) {
		sel, err := resolveSelection(Request{Manifest: manifestWith(markedDefault)})
		require.NoError(t, err)
		assert.Equal(t, "dep-dev", sel.deploymentID)
		assert.Equal(t, "dev", sel.linkName)
	})

	t.Run("a lone marked link is the default too", func(t *testing.T) {
		sel, err := resolveSelection(Request{Manifest: manifestWith(map[string]manifest.Link{
			"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod", Default: true},
		})})
		require.NoError(t, err)
		assert.Equal(t, "dep-prod", sel.deploymentID)
		assert.Equal(t, "prod", sel.linkName)
	})

	t.Run("--deployment beats the marked default", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest:         manifestWith(markedDefault),
			DeploymentID:     "dep-flag",
			ContextWorkspace: "ws-ctx",
		})
		require.NoError(t, err)
		assert.Equal(t, "dep-flag", sel.deploymentID)
		assert.Empty(t, sel.linkName)
	})

	t.Run("a named link beats the marked default", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest: manifestWith(markedDefault),
			LinkName: "prod",
		})
		require.NoError(t, err)
		assert.Equal(t, "dep-prod", sel.deploymentID)
		assert.Equal(t, "prod", sel.linkName)
	})

	t.Run("no default with multiple links falls to unlinked", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest:         manifestWith(twoLinks),
			ContextWorkspace: "ws-ctx",
		})
		require.NoError(t, err)
		assert.Empty(t, sel.deploymentID)
		assert.Equal(t, "ws-ctx", sel.workspaceID)
	})

	// Only an astro link has a Deployment to ship to. Without this guard a
	// non-astro link fell through to the unlinked flow, which prompts for some
	// unrelated Deployment and ships this project's DAGs to it.
	t.Run("a named non-astro link is turned away", func(t *testing.T) {
		_, err := resolveSelection(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"prod":     {Target: "mwaa", Environment: "orders-prod"},
				"dev":      {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
				"scratch":  {URL: "https://airflow.corp.dev"},
				"analysis": {Target: "composer", Environment: "orders-prod"},
			}),
			LinkName: "prod",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `link "prod" is mwaa`)
		assert.Contains(t, err.Error(), "astro links: dev")
	})

	t.Run("the default link is turned away too", func(t *testing.T) {
		_, err := resolveSelection(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"prod": {Target: "composer", Environment: "orders-prod", Default: true},
			}),
			ContextWorkspace: "ws-ctx",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `link "prod" is composer`)
		assert.Contains(t, err.Error(), "no astro links")
	})

	t.Run("an endpoint link is turned away", func(t *testing.T) {
		_, err := resolveSelection(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"staging": {Target: "astro", URL: "https://airflow.staging.corp.dev"},
			}),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint")
	})

	t.Run("no links falls to unlinked", func(t *testing.T) {
		sel, err := resolveSelection(Request{
			Manifest:         manifestWith(nil),
			ContextWorkspace: "ws-ctx",
		})
		require.NoError(t, err)
		assert.Empty(t, sel.deploymentID)
	})
}

// fakeDeployer records calls so tests can assert what the flow drove.
type fakeDeployer struct {
	unlinkedID  string
	unlinkedErr error
	unlinkedWS  string
	resolves    int

	dag      DagResult
	dagErr   error
	dagInput DagDeploy
	deploys  int

	img        ImageResult
	imgErr     error
	imgInput   ImageDeploy
	imgDeploys int
}

func (f *fakeDeployer) ResolveUnlinked(ws string) (string, error) {
	f.resolves++
	f.unlinkedWS = ws
	return f.unlinkedID, f.unlinkedErr
}

func (f *fakeDeployer) DeployDags(in *DagDeploy) (DagResult, error) {
	f.deploys++
	f.dagInput = *in
	return f.dag, f.dagErr
}

func (f *fakeDeployer) DeployImage(in *ImageDeploy) (ImageResult, error) {
	f.imgDeploys++
	f.imgInput = *in
	return f.img, f.imgErr
}

func TestRun_DagsWithImageSourceRejected(t *testing.T) {
	cases := map[string]Request{
		"--dags --image":      {DagsOnly: true, Image: true},
		"--dags --image-name": {DagsOnly: true, ImageName: "my-image:latest"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			d := &fakeDeployer{}
			_, err := Run(req, d)
			require.Error(t, err)
			assert.Zero(t, d.deploys)
			assert.Zero(t, d.imgDeploys)
		})
	}
}

func TestRun_DefaultIsImageAndDag(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{
		WorkspaceID:       "ws-prod",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026",
		DagTarballVersion: "3-169",
		URL:               "https://cloud/deployments/dep-prod",
	}}
	res, err := Run(Request{
		ProjectDir: "/proj",
		Manifest: &manifest.Manifest{
			Project: manifest.Project{Dependencies: []string{"pandas"}},
			Astro: manifest.Astro{
				AirflowVersion: "3.1",
				Packages:       []string{"libpq-dev"},
				Deployments: map[string]manifest.Link{
					"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
				},
			},
		},
	}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.imgDeploys)
	assert.Zero(t, d.deploys)
	// A default deploy ships both, from the manifest fields.
	assert.True(t, d.imgInput.IncludeDags)
	assert.Equal(t, "dep-prod", d.imgInput.DeploymentID)
	assert.Equal(t, "3.1", d.imgInput.AirflowVersion)
	assert.Equal(t, []string{"pandas"}, d.imgInput.Dependencies)
	assert.Equal(t, []string{"libpq-dev"}, d.imgInput.Packages)
	assert.Empty(t, d.imgInput.ImageName)
	assert.Equal(t, "image-and-dag", res.Type)
	assert.Equal(t, "deploy-2026", res.ImageTag)
	assert.Equal(t, "3-169", res.DagTarballVersion)
	assert.Equal(t, "prod", res.LinkName)
}

func TestRun_ImageOnlyDropsDags(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{ImageTag: "deploy-2026"}}
	res, err := Run(Request{
		Image: true,
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
			AirflowVersion: "3.1",
			Deployments: map[string]manifest.Link{
				"prod": {Workspace: "ws-prod", Deployment: "dep-prod"},
			},
		}},
	}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.imgDeploys)
	assert.False(t, d.imgInput.IncludeDags)
	assert.Equal(t, "image-only", res.Type)
	assert.Empty(t, res.DagTarballVersion)
}

func TestRun_ImageNamePassesPrebuiltRef(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{ImageTag: "deploy-2026"}}
	_, err := Run(Request{
		ImageName: "astro-package/demo:3.1-2-abc",
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
			AirflowVersion: "3.1",
			Deployments: map[string]manifest.Link{
				"prod": {Workspace: "ws-prod", Deployment: "dep-prod"},
			},
		}},
	}, d)
	require.NoError(t, err)
	assert.Equal(t, "astro-package/demo:3.1-2-abc", d.imgInput.ImageName)
	// --image-name without --image still ships dags by default.
	assert.True(t, d.imgInput.IncludeDags)
}

func TestRun_ImageTransportErrorPropagates(t *testing.T) {
	sentinel := errors.New("build boom")
	d := &fakeDeployer{imgErr: sentinel}
	_, err := Run(Request{
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
			AirflowVersion: "3.1",
			Deployments: map[string]manifest.Link{
				"prod": {Workspace: "ws-prod", Deployment: "dep-prod"},
			},
		}},
	}, d)
	require.ErrorIs(t, err, sentinel)
}

func TestRun_DefaultLinkDeploysDags(t *testing.T) {
	d := &fakeDeployer{dag: DagResult{
		WorkspaceID:       "ws-prod",
		RuntimeVersion:    "3.1-2",
		DagTarballVersion: "3-169",
		URL:               "https://cloud/deployments/dep-prod",
	}}
	res, err := Run(Request{
		ProjectDir: "/proj",
		DagsOnly:   true,
		Manifest: manifestWith(map[string]manifest.Link{
			"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		}),
	}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.deploys)
	assert.Zero(t, d.resolves)
	assert.Equal(t, "dep-prod", d.dagInput.DeploymentID)
	assert.Equal(t, "/proj", d.dagInput.ProjectDir)
	assert.Equal(t, "dep-prod", res.DeploymentID)
	assert.Equal(t, "ws-prod", res.WorkspaceID)
	assert.Equal(t, "dag-only", res.Type)
	assert.Equal(t, "3.1-2", res.RuntimeVersion)
	assert.Equal(t, "3-169", res.DagTarballVersion)
	assert.Equal(t, "prod", res.LinkName)
}

func TestRun_UnlinkedInteractiveSelects(t *testing.T) {
	d := &fakeDeployer{unlinkedID: "dep-picked", dag: DagResult{WorkspaceID: "ws-ctx"}}
	res, err := Run(Request{
		DagsOnly:         true,
		Interactive:      true,
		ContextWorkspace: "ws-ctx",
		Manifest:         manifestWith(nil),
	}, d)
	require.NoError(t, err)
	assert.Equal(t, 1, d.resolves)
	assert.Equal(t, "ws-ctx", d.unlinkedWS)
	assert.Equal(t, "dep-picked", d.dagInput.DeploymentID)
	assert.Equal(t, "dep-picked", res.DeploymentID)
	assert.Empty(t, res.LinkName)
}

func TestRun_UnlinkedNonInteractiveRequiresDeployment(t *testing.T) {
	d := &fakeDeployer{}
	_, err := Run(Request{
		DagsOnly:         true,
		Interactive:      false,
		ContextWorkspace: "ws-ctx",
		Manifest:         manifestWith(nil),
	}, d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--deployment")
	assert.Zero(t, d.resolves)
	assert.Zero(t, d.deploys)
}

func TestRun_UnlinkedRequiresWorkspace(t *testing.T) {
	d := &fakeDeployer{}
	_, err := Run(Request{
		DagsOnly:    true,
		Interactive: true,
		Manifest:    manifestWith(nil),
	}, d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace")
	assert.Zero(t, d.resolves)
}

func TestRun_TransportErrorPropagates(t *testing.T) {
	sentinel := errors.New("transport boom")
	d := &fakeDeployer{dagErr: sentinel}
	_, err := Run(Request{
		DagsOnly: true,
		Manifest: manifestWith(map[string]manifest.Link{
			"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		}),
	}, d)
	require.ErrorIs(t, err, sentinel)
}
