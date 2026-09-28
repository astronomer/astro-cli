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
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
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

// stubDeployer answers the target prompt with a canned reply, recording what it
// was offered.
type stubDeployer struct {
	fakeDeployer
	answer    string
	answerErr error
	asked     int
	offered   []Choice
	preselect Preselect
}

func (s *stubDeployer) ConfirmTarget(choices []Choice, preselect Preselect) (string, error) {
	s.asked++
	s.offered, s.preselect = choices, preselect
	return s.answer, s.answerErr
}

func choiceNames(choices []Choice) []string {
	names := make([]string, 0, len(choices))
	for _, c := range choices {
		names = append(names, c.Name)
	}
	return names
}

func TestResolveTarget(t *testing.T) {
	oneLink := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
	}
	twoLinks := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		"dev":  {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
	}
	markedDefault := map[string]manifest.Link{
		"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		"dev":  {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev", Default: true},
	}

	t.Run("a named link", func(t *testing.T) {
		d := &stubDeployer{}
		target, err := resolveTarget(Request{Manifest: manifestWith(twoLinks), LinkName: "dev"}, d)
		require.NoError(t, err)
		assert.Equal(t, "dep-dev", target.DeploymentID)
		assert.Equal(t, "ws-dev", target.WorkspaceID)
		assert.Equal(t, "dev", target.LinkName)
		assert.Zero(t, d.asked, "a named target is not a question")
	})

	t.Run("--deployment names a link too", func(t *testing.T) {
		d := &stubDeployer{}
		target, err := resolveTarget(Request{Manifest: manifestWith(twoLinks), Deployment: "prod"}, d)
		require.NoError(t, err)
		assert.Equal(t, "dep-prod", target.DeploymentID)
		assert.Equal(t, "prod", target.LinkName)
		assert.Zero(t, d.asked)
	})

	// --deployment meant a Deployment id before it learned link names, and v1
	// spells the same thing --deployment-id, so a CI job passing one keeps
	// working.
	t.Run("--deployment falls through to a deployment id", func(t *testing.T) {
		d := &stubDeployer{}
		target, err := resolveTarget(Request{
			Manifest:         manifestWith(twoLinks),
			Deployment:       "clx-not-a-link",
			ContextWorkspace: "ws-ctx",
		}, d)
		require.NoError(t, err)
		assert.Equal(t, "clx-not-a-link", target.DeploymentID)
		assert.Equal(t, "ws-ctx", target.WorkspaceID)
		assert.Empty(t, target.LinkName)
	})

	t.Run("--deployment takes --workspace over context", func(t *testing.T) {
		target, err := resolveTarget(Request{
			Deployment:       "dep-flag",
			WorkspaceID:      "ws-flag",
			ContextWorkspace: "ws-ctx",
		}, &stubDeployer{})
		require.NoError(t, err)
		assert.Equal(t, "ws-flag", target.WorkspaceID)
	})

	t.Run("--workspace overrides a link's workspace", func(t *testing.T) {
		target, err := resolveTarget(Request{
			Manifest:    manifestWith(oneLink),
			LinkName:    "prod",
			WorkspaceID: "ws-flag",
		}, &stubDeployer{})
		require.NoError(t, err)
		assert.Equal(t, "ws-flag", target.WorkspaceID)
	})

	// astronomer/deploy-action runs `astro deploy <id>`, so the positional
	// falls through to an id exactly as --deployment does.
	t.Run("the positional falls through to a deployment id", func(t *testing.T) {
		d := &stubDeployer{}
		target, err := resolveTarget(Request{
			Manifest:         manifestWith(twoLinks),
			LinkName:         "clx-not-a-link",
			ContextWorkspace: "ws-ctx",
		}, d)
		require.NoError(t, err)
		assert.Equal(t, "clx-not-a-link", target.DeploymentID)
		assert.Equal(t, "ws-ctx", target.WorkspaceID)
		assert.Empty(t, target.LinkName)
		assert.Zero(t, d.asked)
	})

	t.Run("the positional and --deployment may name the same id", func(t *testing.T) {
		target, err := resolveTarget(Request{LinkName: "clx-id", Deployment: "clx-id"}, &stubDeployer{})
		require.NoError(t, err)
		assert.Equal(t, "clx-id", target.DeploymentID)
	})

	t.Run("two targets named at once", func(t *testing.T) {
		_, err := resolveTarget(Request{
			Manifest:   manifestWith(twoLinks),
			LinkName:   "dev",
			Deployment: "prod",
		}, &stubDeployer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name one")
	})

	// The heart of an earlier fix: the default marker moves the cursor, and the
	// question is asked anyway.
	t.Run("the default link preselects the prompt and never skips it", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		target, err := resolveTarget(Request{Manifest: manifestWith(markedDefault), Interactive: true}, d)
		require.NoError(t, err)
		assert.Equal(t, 1, d.asked)
		assert.Equal(t, Preselect{Name: "dev", From: DefaultMarker}, d.preselect)
		assert.Equal(t, []string{"dev", "prod"}, choiceNames(d.offered))
		assert.Equal(t, "astro deployment dep-dev", d.offered[0].Where)
		assert.Equal(t, "dep-prod", target.DeploymentID, "the answer decides, not the marker")
		assert.Equal(t, "prod", target.LinkName)
	})

	t.Run("a lone link is asked about too", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		target, err := resolveTarget(Request{Manifest: manifestWith(oneLink), Interactive: true}, d)
		require.NoError(t, err)
		assert.Equal(t, 1, d.asked)
		assert.Equal(t, Preselect{Name: "prod", From: DefaultMarker}, d.preselect)
		assert.Equal(t, "dep-prod", target.DeploymentID)
	})

	// ASTRO_DEPLOYMENT and the pin arrive as Preselect. They outrank the marker
	// for the cursor and decide nothing.
	// The ambient layers outrank the marker for the cursor, and the label has to
	// say so: a highlight an exported variable put there is not the file's
	// default, and calling it one tells the reader their manifest says something
	// it does not.
	t.Run("the ambient layers only preselect, and say so", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{
			Manifest:      manifestWith(markedDefault),
			Preselect:     "prod",
			PreselectFrom: "ASTRO_DEPLOYMENT",
			Interactive:   true,
		}, d)
		require.NoError(t, err)
		assert.Equal(t, Preselect{Name: "prod", From: "ASTRO_DEPLOYMENT"}, d.preselect)
	})

	t.Run("a pin is labeled as a pin", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{
			Manifest:      manifestWith(markedDefault),
			Preselect:     "prod",
			PreselectFrom: PinnedBy,
			Interactive:   true,
		}, d)
		require.NoError(t, err)
		assert.Equal(t, Preselect{Name: "prod", From: PinnedBy}, d.preselect)
	})

	t.Run("a preselect naming nothing deployable highlights nothing", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{
			Manifest:    manifestWith(twoLinks),
			Preselect:   "gone",
			Interactive: true,
		}, d)
		require.NoError(t, err)
		assert.Equal(t, Preselect{}, d.preselect)
	})

	t.Run("an aborted prompt deploys nothing", func(t *testing.T) {
		d := &stubDeployer{}
		_, err := resolveTarget(Request{Manifest: manifestWith(twoLinks), Interactive: true}, d)
		require.ErrorIs(t, err, ErrAborted)
	})

	t.Run("a prompt failure travels up", func(t *testing.T) {
		sentinel := errors.New("stdin closed")
		d := &stubDeployer{answerErr: sentinel}
		_, err := resolveTarget(Request{Manifest: manifestWith(twoLinks), Interactive: true}, d)
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("non-interactive with nothing named names the fix", func(t *testing.T) {
		d := &stubDeployer{}
		_, err := resolveTarget(Request{Manifest: manifestWith(markedDefault), Preselect: "prod"}, d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must name the deployment")
		assert.Contains(t, err.Error(), "--deployment <name>")
		assert.Contains(t, err.Error(), "ASTRO_DEPLOYMENT")
		assert.Contains(t, err.Error(), "dev, prod")
		assert.Zero(t, d.asked)
	})

	// Only an astro link has a Deployment to ship to. Without this guard a
	// non-astro link fell through to the unlinked flow, which prompts for some
	// unrelated Deployment and ships this project's DAGs to it.
	t.Run("a named non-astro link is turned away", func(t *testing.T) {
		_, err := resolveTarget(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"prod":     {Target: "mwaa", Environment: "orders-prod"},
				"dev":      {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
				"scratch":  {URL: "https://airflow.corp.dev"},
				"analysis": {Target: "composer", Environment: "orders-prod"},
			}),
			LinkName: "prod",
		}, &stubDeployer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `link "prod" is mwaa`)
		assert.Contains(t, err.Error(), "astro links: dev")
	})

	t.Run("non-astro links are never offered", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"prod":    {Target: "mwaa", Environment: "orders-prod"},
				"dev":     {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
				"scratch": {URL: "https://airflow.corp.dev"},
			}),
			Interactive: true,
		}, d)
		require.NoError(t, err)
		assert.Equal(t, []string{"dev"}, choiceNames(d.offered))
	})

	t.Run("a project whose links are all unshippable", func(t *testing.T) {
		d := &stubDeployer{}
		_, err := resolveTarget(Request{
			Manifest: manifestWith(map[string]manifest.Link{
				"prod":    {Target: "composer", Environment: "orders-prod", Default: true},
				"scratch": {URL: "https://airflow.corp.dev"},
			}),
			Interactive:      true,
			ContextWorkspace: "ws-ctx",
		}, d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "this project links none")
		assert.Contains(t, err.Error(), "prod is composer")
		assert.Contains(t, err.Error(), "scratch is endpoint")
		assert.Zero(t, d.asked)
		assert.Zero(t, d.resolves, "the workspace-level flow would offer an unrelated Deployment")
	})
}

// fakeDeployer records calls so tests can assert what the flow drove.
type fakeDeployer struct {
	answer      string
	answerErr   error
	confirms    int
	offered     []Choice
	preselected Preselect

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

func (f *fakeDeployer) ConfirmTarget(choices []Choice, preselect Preselect) (string, error) {
	f.confirms++
	f.offered, f.preselected = choices, preselect
	return f.answer, f.answerErr
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
		LinkName:   "prod",
		Manifest: &manifest.Manifest{
			Project: manifest.Project{Dependencies: []string{"apache-airflow==3.1.*", "pandas"}},
			Astro: manifest.Astro{
				Packages: []string{"libpq-dev"},
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
	assert.Equal(t, []string{"apache-airflow==3.1.*", "pandas"}, d.imgInput.Dependencies)
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
		Image:    true,
		LinkName: "prod",
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
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
		LinkName:  "prod",
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
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
		LinkName: "prod",
		Manifest: &manifest.Manifest{Astro: manifest.Astro{
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
		LinkName:   "prod",
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

// Announce is what carries the "building your project image" line, so it must
// land after the target is settled and never at all when nobody settled one.
func TestRun_AnnounceFiresOnlyOnceTheTargetIsSettled(t *testing.T) {
	t.Run("after the answer", func(t *testing.T) {
		d := &fakeDeployer{answer: "prod"}
		var announced []Target
		_, err := Run(Request{
			Interactive: true,
			Manifest: manifestWith(map[string]manifest.Link{
				"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
			}),
			Announce: func(target Target) { announced = append(announced, target) },
		}, d)
		require.NoError(t, err)
		require.Len(t, announced, 1)
		assert.Equal(t, "dep-prod", announced[0].DeploymentID)
		assert.Equal(t, "prod", announced[0].LinkName)
	})

	t.Run("not on a refusal", func(t *testing.T) {
		d := &fakeDeployer{}
		announced := 0
		_, err := Run(Request{
			Interactive: true,
			Manifest: manifestWith(map[string]manifest.Link{
				"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
				"dev":  {Target: "astro", Workspace: "ws-dev", Deployment: "dep-dev"},
			}),
			Announce: func(Target) { announced++ },
		}, d)
		require.Error(t, err)
		assert.Zero(t, announced)
	})
}

func TestRun_TransportErrorPropagates(t *testing.T) {
	sentinel := errors.New("transport boom")
	d := &fakeDeployer{dagErr: sentinel}
	_, err := Run(Request{
		DagsOnly: true,
		LinkName: "prod",
		Manifest: manifestWith(map[string]manifest.Link{
			"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
		}),
	}, d)
	require.ErrorIs(t, err, sentinel)
}

// The manifest's declared Dockerfile reaches the transport.
//
// This is the layer that reads the manifest, and it had no test: a mutant that
// stopped carrying the field survived the whole internal/deploy suite, because
// the cloud/deploy tests construct ImageDeployV2Input directly and never
// exercise runImage. Without the carry a tier-3 project deploys a generated
// image with its own build silently dropped.
//
// The absent case is asserted too, so the field cannot be filled from something
// other than the manifest and still pass.
func TestRun_CarriesTheDeclaredDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared string
	}{
		{"declared", "docker/Dockerfile"},
		{"not declared", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &fakeDeployer{img: ImageResult{WorkspaceID: "ws-prod", RuntimeVersion: "3.1-2"}}
			_, err := Run(Request{
				ProjectDir: "/proj",
				LinkName:   "prod",
				Manifest: &manifest.Manifest{
					Project: manifest.Project{Dependencies: []string{"pandas"}},
					Astro: manifest.Astro{
						Dockerfile: tc.declared,
						Deployments: map[string]manifest.Link{
							"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
						},
					},
				},
			}, d)
			require.NoError(t, err)
			assert.Equal(t, tc.declared, d.imgInput.Dockerfile)
		})
	}
}

// --build-secret reaches the image build when the project declares a Dockerfile.
//
// This replaces a blanket refusal of the flag on every v2 project. That refusal
// was right about the rule and wrong about the reason: it said a v2 project
// "cannot declare" a Dockerfile, which stopped being true when [tool.astro]
// dockerfile landed.
func TestRun_CarriesBuildSecretsWithADeclaredDockerfile(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{WorkspaceID: "ws-prod", RuntimeVersion: "3.1-2"}}
	_, err := Run(Request{
		ProjectDir:   "/proj",
		LinkName:     "prod",
		BuildSecrets: []string{"id=pypi,src=/tmp/pypi.txt"},
		Manifest: &manifest.Manifest{
			Astro: manifest.Astro{
				Dockerfile: "Dockerfile",
				Deployments: map[string]manifest.Link{
					"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
				},
			},
		},
	}, d)
	require.NoError(t, err)
	assert.Equal(t, []string{"id=pypi,src=/tmp/pypi.txt"}, d.imgInput.BuildSecrets)
}

// Secrets are CARRIED without a Dockerfile, not refused.
//
// The refusal moved to cmd/cloud, and this pins why. ResolveBuildSecrets also
// reads BUILD_SECRET_INPUT from the environment, so a refusal here — which
// cannot tell a flag from an ambient variable — turned an exported
// BUILD_SECRET_INPUT into a hard failure for every project that generates its
// image, with no flag given and a message telling the user to declare a
// Dockerfile they never wanted. imagebuild drops them in generated mode anyway.
func TestRun_CarriesBuildSecretsEvenWithoutADockerfile(t *testing.T) {
	d := &fakeDeployer{img: ImageResult{WorkspaceID: "ws-prod", RuntimeVersion: "3.1-2"}}
	_, err := Run(Request{
		ProjectDir:   "/proj",
		LinkName:     "prod",
		BuildSecrets: []string{"id=pypi"},
		Manifest: &manifest.Manifest{
			Astro: manifest.Astro{
				Deployments: map[string]manifest.Link{
					"prod": {Target: "astro", Workspace: "ws-prod", Deployment: "dep-prod"},
				},
			},
		},
	}, d)
	require.NoError(t, err, "an ambient BUILD_SECRET_INPUT must not fail a deploy that asked for nothing")
	assert.Equal(t, 1, d.imgDeploys)
}
