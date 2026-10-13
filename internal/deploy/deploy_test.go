package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
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
	preselect string
}

func (s *stubDeployer) ConfirmTarget(choices []Choice, preselect string) (string, error) {
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

	// --deployment meant a Deployment id before it learned link names, and the 1.x path
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

	// The heart of "deploy always asks": the default marker moves the cursor, and the
	// question is asked anyway.
	t.Run("the default link preselects the prompt and never skips it", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		target, err := resolveTarget(Request{Manifest: manifestWith(markedDefault), Interactive: true}, d)
		require.NoError(t, err)
		assert.Equal(t, 1, d.asked)
		assert.Equal(t, "dev", d.preselect)
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
		assert.Equal(t, "prod", d.preselect)
		assert.Equal(t, "dep-prod", target.DeploymentID)
	})

	// Only the committed default moves the cursor. ASTRO_DEPLOYMENT and the
	// `astro use` selection pick which Airflow the query commands read; where
	// code ships is the project's, so they never reach the prompt.
	t.Run("only the default preselects, whatever the shell says", func(t *testing.T) {
		t.Setenv(deploymentEnvVar, "prod")
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{Manifest: manifestWith(markedDefault), Interactive: true}, d)
		require.NoError(t, err)
		assert.Equal(t, "dev", d.preselect)
	})

	t.Run("several links and no default highlight nothing", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		_, err := resolveTarget(Request{Manifest: manifestWith(twoLinks), Interactive: true}, d)
		require.NoError(t, err)
		assert.Empty(t, d.preselect)
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
		_, err := resolveTarget(Request{Manifest: manifestWith(markedDefault)}, d)
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
	preselected string

	unlinkedID  string
	unlinkedErr error
	unlinkedWS  string
	resolves    int

	// listed is what ListDeployments returns for any workspace.
	listed    []Deployment
	listErr   error
	listedWS  string
	listCalls int

	dag      DagResult
	dagErr   error
	dagInput DagDeploy
	deploys  int

	img        ImageResult
	imgErr     error
	imgInput   ImageDeploy
	imgDeploys int
}

func (f *fakeDeployer) ConfirmTarget(choices []Choice, preselect string) (string, error) {
	f.confirms++
	f.offered, f.preselected = choices, preselect
	return f.answer, f.answerErr
}

func (f *fakeDeployer) ListDeployments(ws string) ([]Deployment, error) {
	f.listCalls++
	f.listedWS = ws
	return f.listed, f.listErr
}

func (f *fakeDeployer) ResolveUnlinked(ws string) (Deployment, error) {
	f.resolves++
	f.unlinkedWS = ws
	return Deployment{ID: f.unlinkedID, Name: "Picked One"}, f.unlinkedErr
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
		Git:               Git{Commit: &Commit{SHA: "abc123", Branch: "main"}},
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
	assert.Equal(t, "3.1", d.imgInput.Build.AirflowVersion)
	assert.Equal(t, []string{"apache-airflow==3.1.*", "pandas"}, d.imgInput.Build.Dependencies)
	assert.Equal(t, []string{"libpq-dev"}, d.imgInput.Build.Packages)
	assert.Empty(t, d.imgInput.ImageName)
	assert.Equal(t, "image-and-dag", res.Type)
	assert.Equal(t, "deploy-2026", res.ImageTag)
	assert.Equal(t, "3-169", res.DagTarballVersion)
	assert.Equal(t, "prod", res.LinkName)
	assert.Equal(t, &Commit{SHA: "abc123", Branch: "main"}, res.Git.Commit)
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
		Git:               Git{Uncommitted: true},
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
	assert.True(t, res.Git.Uncommitted)
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
	assert.Empty(t, res.LinkName, "with no Link hook, nothing is linked")
}

// A project that links nothing links what the workspace-level flow picks, named
// after the Deployment.
func TestRun_UnlinkedPickIsLinked(t *testing.T) {
	d := &fakeDeployer{unlinkedID: "dep-picked"}
	var saved []scaffold.Link
	res, err := Run(Request{
		DagsOnly:         true,
		Interactive:      true,
		ContextWorkspace: "ws-ctx",
		Manifest:         manifestWith(nil),
		Link:             func(l scaffold.Link) error { saved = append(saved, l); return nil },
	}, d)
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, scaffold.Link{Name: "picked-one", Kind: manifest.KindAstro, Workspace: "ws-ctx", Deployment: "dep-picked"}, saved[0])
	assert.Equal(t, "picked-one", res.LinkName)
}

// A project that links nothing lists the workspace it declares, as the prompt
// with links does, before the login's.
func TestRun_UnlinkedUsesTheProjectsWorkspace(t *testing.T) {
	d := &fakeDeployer{unlinkedID: "dep-picked"}
	m := manifestWith(nil)
	m.Astro.Workspace = "ws-project"
	_, err := Run(Request{DagsOnly: true, Interactive: true, ContextWorkspace: "ws-ctx", Manifest: m}, d)
	require.NoError(t, err)
	assert.Equal(t, "ws-project", d.unlinkedWS)
}

func TestResolveTarget_OffersTheWorkspacesUnlinkedDeployments(t *testing.T) {
	links := map[string]manifest.Link{
		"prod":    {Target: "astro", Workspace: "ws-1", Deployment: "dep-prod"},
		"dev":     {Target: "astro", Workspace: "ws-1", Deployment: "dep-dev", Default: true},
		"staging": {Target: "astro", Workspace: "ws-1", Deployment: "dep-staging"},
	}
	listed := []Deployment{
		{ID: "dep-prod", Name: "Prod"},                              // linked: not offered again
		{ID: "dep-new", Name: "Orders (EU)"},                        // offered as orders-eu
		{ID: "dep-clash", Name: "Staging"},                          // "staging" is a link already
		{ID: "dep-local", Name: "Local"},                            // "local" is reserved
		{ID: "dep-other", Name: "Orders (EU)", WorkspaceID: "ws-1"}, // same name twice
	}
	req := Request{Interactive: true, Manifest: manifestWith(links), ContextWorkspace: "ws-1"}

	t.Run("links first, the default on top, then the rest", func(t *testing.T) {
		d := &stubDeployer{answer: "dev"}
		d.listed = listed
		_, err := resolveTarget(req, d)
		require.NoError(t, err)
		assert.Equal(t, "ws-1", d.listedWS)
		assert.Equal(t, []string{"dev", "prod", "staging", "dep-local", "orders-eu", "orders-eu-2", "staging-2"}, choiceNames(d.offered))
		for _, c := range d.offered {
			assert.Equal(t, !slices.Contains([]string{"dev", "prod", "staging"}, c.Name), c.Unlinked, c.Name)
		}
		assert.Equal(t, "dev", d.preselect)
	})

	t.Run("a Deployment named like a row number is listed by id", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		d.listed = []Deployment{{ID: "dep-one", Name: "1"}}
		_, err := resolveTarget(req, d)
		require.NoError(t, err)
		assert.Equal(t, []string{"dev", "prod", "staging", "dep-one"}, choiceNames(d.offered))
	})

	t.Run("picking an unlinked Deployment links it once the deploy succeeds", func(t *testing.T) {
		d := &stubDeployer{answer: "orders-eu"}
		d.listed = listed
		var saved []scaffold.Link
		r := req
		r.DagsOnly = true
		r.Link = func(l scaffold.Link) error {
			assert.Equal(t, 1, d.deploys, "saved only after the deploy")
			saved = append(saved, l)
			return nil
		}
		res, err := Run(r, d)
		require.NoError(t, err)
		assert.Equal(t, "dep-new", d.dagInput.DeploymentID)
		assert.Equal(t, "orders-eu", res.LinkName)
		require.Len(t, saved, 1)
		assert.Equal(t, scaffold.Link{Name: "orders-eu", Kind: manifest.KindAstro, Workspace: "ws-1", Deployment: "dep-new"}, saved[0])
	})

	t.Run("a deploy that fails saves no link", func(t *testing.T) {
		d := &stubDeployer{answer: "orders-eu"}
		d.listed = listed
		d.dagErr = errors.New("upload failed")
		r := req
		r.DagsOnly = true
		r.Link = func(scaffold.Link) error { t.Fatal("a failed deploy saved a link"); return nil }
		_, err := Run(r, d)
		require.Error(t, err)
	})

	t.Run("a link that cannot be saved warns, and the deploy still succeeds", func(t *testing.T) {
		d := &stubDeployer{answer: "staging-2"}
		d.listed = listed
		var warned []string
		r := req
		r.DagsOnly = true
		r.Warn = func(s string) { warned = append(warned, s) }
		r.Link = func(scaffold.Link) error { return errors.New("read-only") }
		res, err := Run(r, d)
		require.NoError(t, err)
		assert.Equal(t, "dep-clash", res.DeploymentID)
		assert.Empty(t, res.LinkName)
		require.Len(t, warned, 1)
		assert.Contains(t, warned[0], "read-only")
	})

	t.Run("a list that fails still offers the links", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		d.listErr = errors.New("offline")
		var warned []string
		r := req
		r.Warn = func(s string) { warned = append(warned, s) }
		target, err := resolveTarget(r, d)
		require.NoError(t, err)
		assert.Equal(t, "prod", target.LinkName)
		assert.Equal(t, []string{"dev", "prod", "staging"}, choiceNames(d.offered))
		require.Len(t, warned, 1)
		assert.Contains(t, warned[0], "offline")
	})

	t.Run("--workspace picks the workspace listed", func(t *testing.T) {
		d := &stubDeployer{answer: "prod"}
		r := req
		r.WorkspaceID = "ws-flag"
		_, err := resolveTarget(r, d)
		require.NoError(t, err)
		assert.Equal(t, "ws-flag", d.listedWS)
	})

	t.Run("a run that cannot be asked lists nothing", func(t *testing.T) {
		d := &stubDeployer{}
		r := req
		r.Interactive = false
		_, err := resolveTarget(r, d)
		require.Error(t, err)
		assert.Zero(t, d.listCalls)
	})
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
// the cloud/deploy tests construct ManifestImageDeployInput directly and never
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
			assert.Equal(t, tc.declared, d.imgInput.Build.Dockerfile)
		})
	}
}

// --build-secret reaches the image build when the project declares a Dockerfile.
//
// This replaces a blanket refusal of the flag on every project. That refusal
// was right about the rule and wrong about the reason: it said a project
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
