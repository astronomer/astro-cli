package deploy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// fakeEngineCmd is the one runner every image action of a manifest deploy
// goes through: the engine check, the build, the label reads and the push.
// It touches no daemon.
type fakeEngineCmd struct {
	calls []string
	// dockerfiles are the Dockerfiles each build was given, as they read
	// while it ran.
	dockerfiles []string
	// label answers a label read of an image.
	label func(image, label string) string
	// pushAuth is the credential file the last push ran with, as it read
	// while the push ran: "" when the push carried none.
	pushAuth string
}

func (f *fakeEngineCmd) Run(_ context.Context, env []string, stdio localrt.Stdio, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	switch {
	case len(args) > 0 && args[0] == "push":
		f.pushAuth = ""
		for _, kv := range env {
			if dir, ok := strings.CutPrefix(kv, "DOCKER_CONFIG="); ok {
				data, err := os.ReadFile(filepath.Join(dir, "config.json"))
				if err != nil {
					return err
				}
				f.pushAuth = string(data)
			}
		}
	case len(args) > 3 && args[0] == "image" && args[1] == "inspect" && args[3] == "{{json .RepoDigests}}" && stdio.Out != nil:
		image := args[len(args)-1]
		repo := image[:strings.LastIndex(image, ":")]
		fmt.Fprintf(stdio.Out, "[%q]\n", repo+"@sha256:abc")
	case len(args) > 0 && args[0] == "build":
		if i := slices.Index(args, "--file"); i >= 0 {
			data, err := os.ReadFile(args[i+1])
			if err != nil {
				return err
			}
			f.dockerfiles = append(f.dockerfiles, string(data))
		}
	case len(args) > 3 && args[0] == "image" && args[1] == "inspect" && stdio.Out != nil:
		if f.label != nil {
			format := args[3]
			label := format[strings.Index(format, `"`)+1 : strings.LastIndex(format, `"`)]
			fmt.Fprintln(stdio.Out, f.label(args[len(args)-1], label))
		}
	case len(args) == 1 && args[0] == "--version" && stdio.Out != nil:
		fmt.Fprintf(stdio.Out, "%s version 27.0.0\n", name)
	}
	return nil
}

// called reports whether a call containing part ran.
func (f *fakeEngineCmd) called(part string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.Contains(c, part) })
}

// withManifestEngine wires a manifest deploy's engine to a fake whose images
// carry runtime, for one test.
func withManifestEngine(t *testing.T, runtime string) *fakeEngineCmd {
	t.Helper()
	// No push credential lands in, or is read from, the developer's docker
	// config.
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	cmd := &fakeEngineCmd{label: func(_, label string) string {
		switch label {
		case "io.astronomer.docker.runtime.version":
			return runtime
		case airflowImageLabel:
			return "3.1.1"
		}
		return ""
	}}
	prevCmd, prevNow := newManifestCommander, manifestBuildNow
	newManifestCommander = func() imagebuild.Commander { return cmd }
	manifestBuildNow = func() time.Time { return time.Unix(0, 0) }
	t.Cleanup(func() { newManifestCommander, manifestBuildNow = prevCmd, prevNow })
	return cmd
}

// apcTarget is the Houston a manifest deploy talks to: a workspace "ws"
// holding the Deployment "dep" (release my-release, next tag deploy-7) of
// the DAG type given, on a cluster that lists listed, at platform version.
type apcTarget struct {
	dagType  string
	current  string
	listed   houston.RuntimeReleases
	version  string
	byo      bool
	dagsHook string
}

func (a *apcTarget) client(t *testing.T) *houston_mocks.ClientInterface {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	c := new(houston_mocks.ClientInterface)
	c.On("GetWorkspace", "ws").Return(&houston.Workspace{ID: "ws"}, nil).Maybe()
	c.On("ListDeployments", mock.Anything).Return([]houston.Deployment{
		{ID: "dep", ReleaseName: "my-release", DeploymentInfo: houston.DeploymentInfo{NextCli: "deploy-7"}, Workspace: houston.Workspace{ID: "ws"}},
	}, nil).Maybe()
	urls := []houston.DeploymentURL{
		{Type: houston.AirflowURLType, URL: "https://deployments.example.com/my-release/airflow"},
		{Type: houston.RegistryURLType, URL: "registry.example.com"},
	}
	if a.dagsHook != "" {
		urls = append(urls, houston.DeploymentURL{Type: houston.DagServerURLType, URL: a.dagsHook})
	}
	c.On("GetDeployment", "dep").Return(&houston.Deployment{
		ID: "dep", ReleaseName: "my-release", ClusterID: "cl", RuntimeVersion: a.current, Urls: urls,
		DagDeployment: houston.DagDeploymentConfig{Type: a.dagType}, DagDeploymentRead: true, Workspace: houston.Workspace{ID: "ws"},
	}, nil).Maybe()
	cfg := &houston.AppConfig{Version: a.version, Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
	if a.byo {
		cfg.Flags.BYORegistryEnabled, cfg.BYORegistryDomain = true, "byo.example.com/team/repo"
	}
	c.On("GetAppConfig", mock.Anything).Return(cfg, nil).Maybe()
	c.On("GetRuntimeReleases", map[string]interface{}{"clusterId": "cl", "workspaceUuid": "ws", "deploymentUuid": "dep"}).Return(a.listed, nil).Maybe()
	c.On("GetPlatformVersion", mock.Anything).Return(a.version, nil).Maybe()
	return c
}

// houstonLists is what a Houston with Airflow 3 on lists: the newest build of
// each Airflow version.
var houstonLists = houston.RuntimeReleases{
	{Version: "3.1-5", AirflowVersion: "3.1.1"},
	{Version: "3.0-9", AirflowVersion: "3.0.6"},
	{Version: "12.1.1", AirflowVersion: "2.10.3"},
}

// projectWithDags is a project root with one DAG.
func projectWithDags(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "example.py"), []byte("# dag\n"), 0o600))
	return dir
}

func engineResolved() (bin string, env []string, err error) { return "docker", nil, nil }

func manifestImageFor(t *testing.T, build *imagebuild.ManifestBuild, warnings *[]string) *ManifestImage {
	t.Helper()
	if build.ProjectDir == "" {
		build.ProjectDir = projectWithDags(t)
	}
	return &ManifestImage{
		DeploymentID: "dep",
		WorkspaceID:  "ws",
		Build:        *build,
		IncludeDags:  true,
		Engine:       engineResolved,
		Description:  "ship it",
		Progress:     io.Discard,
		Warn:         func(w string) { *warnings = append(*warnings, w) },
		Catalog:      runtimeversions.Options{CacheDir: t.TempDir(), Timeout: time.Nanosecond},
	}
}

// A generated build to an image Deployment: built at linux/amd64 on the
// newest runtime Houston lists for the pin, with the labels Houston reads,
// carrying the project's DAGs, and pushed to the Deployment's registry under
// the release's repository with the context's token, carried by the push
// alone. Nothing is uploaded.
func TestDeployManifestImageToAnImageDeployment(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.0-9", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)

	assert.Equal(t, ManifestImageResult{
		WorkspaceID:    "ws",
		RuntimeVersion: "3.1-5",
		Image:          "registry.example.com/my-release/airflow:deploy-7",
		URL:            "https://deployments.example.com/my-release/airflow",
		DagsInImage:    true,
	}, res)
	assert.Empty(t, warnings)
	require.Len(t, cmd.dockerfiles, 1)
	assert.Equal(t, "FROM astrocrpublic.azurecr.io/runtime:3.1-5\n", cmd.dockerfiles[0], "the newest build Houston lists for the pin")
	assert.True(t, cmd.called("--platform linux/amd64"), "%v", cmd.calls)
	assert.True(t, cmd.called("--label io.astronomer.skip.revision=true --label io.astronomer.deploy.revision.description=ship it"), "%v", cmd.calls)
	assert.True(t, cmd.called("docker push registry.example.com/my-release/airflow:deploy-7"), "%v", cmd.calls)
	assert.False(t, cmd.called("docker login"), "the push carries its own credential: %v", cmd.calls)
}

// An Airflow 2 project builds on Astro Runtime for Airflow 2, the newest build
// Houston lists for its pin, as astro local does.
func TestDeployManifestImageAirflow2(t *testing.T) {
	cmd := withManifestEngine(t, "12.1.1")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "12.0.0", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "2.10"}, &warnings))
	require.NoError(t, err)
	assert.Equal(t, "12.1.1", res.RuntimeVersion)
	require.Len(t, cmd.dockerfiles, 1)
	assert.Equal(t, "FROM quay.io/astronomer/astro-runtime:12.1.1\n", cmd.dockerfiles[0])
}

// Airflow 2 to 3 is an upgrade Houston allows, from any Airflow 2 runtime,
// and the deploy says so.
func TestDeployManifestImageUpgradesAirflow2To3(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "11.9.0", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	assert.True(t, res.Airflow3Upgrade)
	assert.Equal(t, []string{"this deploy upgrades Deployment dep from Airflow 2 to Airflow 3 (Astro Runtime 11.9.0 to 3.1-5)"}, warnings)
}

// What Houston would drop after the push, where the deploy never hears of
// it, is refused before anything is built: a downgrade, Airflow 3 to 2
// among them, and a runtime newer than the newest it lists.
func TestDeployManifestImageRefusesWhatHoustonWouldDrop(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		build   imagebuild.ManifestBuild
		wantErr string
	}{
		{"a downgrade", "3.1-5", imagebuild.ManifestBuild{AirflowVersion: "3.0"}, "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5"},
		{"Airflow 3 to 2", "3.0-9", imagebuild.ManifestBuild{AirflowVersion: "2.10"}, "cannot deploy Astro Runtime 12.1.1: it is a downgrade from the deployment's current 3.0-9"},
		{"newer than the newest listed", "3.0-9", imagebuild.ManifestBuild{AirflowVersion: "3.2", Runtime: "3.2-1"}, "it is newer than the newest the platform lists for this Deployment, 3.1-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := withManifestEngine(t, "irrelevant")
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: tc.current, listed: houstonLists, version: "1.0.0"}).client(t)
			var warnings []string
			_, err := DeployManifestImage(client, manifestImageFor(t, &tc.build, &warnings))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.False(t, cmd.called(" build "), "nothing is built: %v", cmd.calls)
			assert.False(t, cmd.called(" push "), "nothing is pushed: %v", cmd.calls)
		})
	}
}

// A dag_deploy Deployment takes the DAGs as an upload beside the image.
func TestDeployManifestImageUploadsDagsToADagDeployDeployment(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	var uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: server.URL}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	assert.True(t, res.DagsUploaded)
	assert.False(t, res.DagsInImage)
	assert.Equal(t, 1, uploads)
}

// A git-sync Deployment takes its DAGs from its repository: the image ships
// alone, and the deploy says why.
func TestDeployManifestImageToAGitSyncDeployment(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.GitSyncDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	assert.True(t, res.DagsNotShipped)
	assert.Equal(t, []string{"Deployment dep takes its Dags from a git repository (git-sync), so this deploy shipped the image alone and the Dags are as they were"}, warnings)
}

// --image to an image Deployment would leave it no DAGs: refused before the
// build.
func TestDeployManifestImageRefusesImageOnlyToAnImageDeployment(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings)
	in.IncludeDags = false
	_, err := DeployManifestImage(client, in)
	require.ErrorIs(t, err, ErrDeploymentTypeIncorrectForImageOnly)
	assert.False(t, cmd.called(" build "))
}

// A BYO registry takes the push on the engine's own login, and the image by
// the updateDeploymentImage mutation, with the runtime the image carries.
func TestDeployManifestImageToABYORegistry(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", byo: true}).client(t)
	client.On("UpdateDeploymentImage", mock.MatchedBy(func(r houston.UpdateDeploymentImageRequest) bool {
		return r.ReleaseName == "my-release" && r.RuntimeVersion == "3.1-5" && strings.HasPrefix(r.Image, "byo.example.com/team/repo:my-release-deploy-")
	})).Return(nil, nil).Once()
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.Image, "byo.example.com/team/repo:my-release-deploy-"), res.Image)
	assert.True(t, cmd.called("docker push byo.example.com/team/repo:my-release-deploy-"), "%v", cmd.calls)
	client.AssertExpectations(t)
}

// Below Houston 1.0 the registry is registry.<domain>.
func TestDeployManifestImageBelow1(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "0.37.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	assert.Equal(t, "registry."+c.Domain+"/my-release/airflow:deploy-7", res.Image)
	assert.True(t, cmd.called("docker push registry."+c.Domain+"/my-release/airflow:deploy-7"), "%v", cmd.calls)
}

// A Deployment that is not in the workspace is refused, naming both.
func TestDeployManifestImageRefusesADeploymentOutsideTheWorkspace(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings)
	in.DeploymentID = "elsewhere"
	_, err := DeployManifestImage(client, in)
	require.EqualError(t, err, "no Deployment elsewhere in workspace ws: name a Deployment of the workspace, or the workspace it is in with --workspace")
}

// The base a generated build starts FROM: the manifest's runtime, else the
// newest build Houston lists for the pin, by local Docker mode's rule; an
// Airflow 3 pin Houston lists nothing for builds on its series tag, and an
// Airflow 2 one on the public catalog's newest, refused with the fix when
// that cannot be read either.
func TestHoustonBase(t *testing.T) {
	offline := runtimeversions.Options{CacheDir: t.TempDir(), Timeout: time.Nanosecond}
	for _, tc := range []struct {
		name    string
		build   imagebuild.ManifestBuild
		listed  houston.RuntimeReleases
		want    string
		wantErr string
	}{
		{"the newest of the series Houston lists", imagebuild.ManifestBuild{AirflowVersion: "3.1"}, houstonLists, "astrocrpublic.azurecr.io/runtime:3.1-5", ""},
		{"an exact Airflow version", imagebuild.ManifestBuild{AirflowVersion: "2.10.3"}, houstonLists, "quay.io/astronomer/astro-runtime:12.1.1", ""},
		{"the manifest's runtime wins", imagebuild.ManifestBuild{AirflowVersion: "3.1", Runtime: "3.1-4"}, houstonLists, "astrocrpublic.azurecr.io/runtime:3.1-4", ""},
		{"3.1 is not 3.10", imagebuild.ManifestBuild{AirflowVersion: "3.1"}, houston.RuntimeReleases{{Version: "3.10-1", AirflowVersion: "3.10.0"}}, "astrocrpublic.azurecr.io/runtime:3.1", ""},
		{"nothing listed for Airflow 3", imagebuild.ManifestBuild{AirflowVersion: "3.1"}, nil, "astrocrpublic.azurecr.io/runtime:3.1", ""},
		{"nothing listed for Airflow 2, and no catalog", imagebuild.ManifestBuild{AirflowVersion: "2.10"}, nil, "", "set [tool.astro] runtime in pyproject.toml"},
		{"Airflow 2 below 2.7", imagebuild.ManifestBuild{AirflowVersion: "2.6"}, houston.RuntimeReleases{{Version: "8.8.0", AirflowVersion: "2.6.3"}}, "", "needs Airflow 2.7 or later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := houstonBase(listedReleases(tc.listed), offline)(context.Background(), tc.build)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// The push to the platform registry carries the context's token as the
// registry's user, in the credential file made for that push alone.
func TestDeployManifestImagePushCarriesTheToken(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	require.NotEmpty(t, c.Token)
	var warnings []string
	_, err = DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	want := base64.StdEncoding.EncodeToString([]byte(registryUser + ":" + strings.TrimPrefix(c.Token, "Bearer ")))
	assert.Contains(t, cmd.pushAuth, want, "the push's credential file")
	assert.Contains(t, cmd.pushAuth, "registry.example.com")
}

// A BYO registry takes the push on the engine's own login, with no
// credential of the deploy's, and under sha_as_tag the mutation names the
// image by its digest. The deploy's description goes with the mutation.
func TestDeployManifestImageToABYORegistryByDigest(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", byo: true}).client(t)
	require.NoError(t, config.CFG.ShaAsTag.SetHomeString("true"))
	t.Cleanup(func() { _ = config.CFG.ShaAsTag.SetHomeString("false") })
	client.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{
		ReleaseName: "my-release", Image: "byo.example.com/team/repo@sha256:abc", AirflowVersion: "3.1.1", RuntimeVersion: "3.1-5", DeployRevisionDescription: "ship it",
	}).Return(nil, nil).Once()
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	assert.Equal(t, "byo.example.com/team/repo@sha256:abc", res.Image)
	assert.Empty(t, cmd.pushAuth, "no credential of the deploy's")
	client.AssertExpectations(t)
}

// A runtime the platform does not list, and no newer than the newest it
// does (an older build of a listed Airflow, say), deploys: Houston takes any
// build it knows. The deploy says so.
func TestDeployManifestImageWarnsOfAnUnlistedRuntime(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-4")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.0-9", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1", Runtime: "3.1-4"}, &warnings))
	require.NoError(t, err)
	assert.Equal(t, "3.1-4", res.RuntimeVersion)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "Astro Runtime 3.1-4 is not one the platform lists for this Deployment")
	assert.True(t, cmd.called(" push "))
}

// The built image's label is the authority: one newer than the newest listed
// is refused after the build, though the plan passed, and nothing is pushed.
func TestDeployManifestImageRefusesABuiltRuntimeNewerThanListed(t *testing.T) {
	cmd := withManifestEngine(t, "3.2-1")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.0-9", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot deploy Astro Runtime 3.2-1: it is newer than the newest the platform lists")
	assert.True(t, cmd.called(" build "), "the plan passed")
	assert.False(t, cmd.called(" push "))
}

// An image Deployment runs the DAGs in its image: a generated image whose
// .dockerignore drops every DAG file is refused before the build, and a
// declared Dockerfile's is warned about, since its own steps may copy them.
func TestDeployManifestImageHoldsAnImageDeploymentToDagsInTheImage(t *testing.T) {
	t.Run("a generated build", func(t *testing.T) {
		cmd := withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
		dir := projectWithDags(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))
		var warnings []string
		_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"}, &warnings))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the project's .dockerignore leaves out every Dag file under dags/")
		assert.False(t, cmd.called(" build "))
	})
	t.Run("a declared Dockerfile", func(t *testing.T) {
		cmd := withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
		dir := projectWithDags(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-5\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))
		var warnings []string
		_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"}, &warnings))
		require.NoError(t, err)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "leaves out every Dag file under dags/")
		assert.True(t, cmd.called(" push "))
	})
}

// A Deployment whose DAG source could not be read is uploaded to; one that
// turns out to take no upload ships the image alone, and the deploy says so.
// So does one that takes uploads, from a project with no dags directory.
func TestDeployManifestImageSaysWhyNoDagsShipped(t *testing.T) {
	t.Run("a Deployment whose DAG source was not read takes no upload", func(t *testing.T) {
		withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: "", current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
		unread := &houston.Deployment{ID: "dep", ReleaseName: "my-release", ClusterID: "cl", RuntimeVersion: "3.1-5", Workspace: houston.Workspace{ID: "ws"}, Urls: []houston.DeploymentURL{{Type: houston.RegistryURLType, URL: "registry.example.com"}}}
		client.ExpectedCalls = slices.DeleteFunc(client.ExpectedCalls, func(c *mock.Call) bool { return c.Method == "GetDeployment" })
		client.On("GetDeployment", "dep").Return(unread, nil)
		var warnings []string
		res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
		require.NoError(t, err)
		assert.True(t, res.DagsNotShipped)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "Deployment dep takes no Dag upload")
	})
	t.Run("a dag_deploy Deployment and no dags directory", func(t *testing.T) {
		withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: "https://dags.invalid"}).client(t)
		dir := t.TempDir()
		var warnings []string
		res, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"}, &warnings))
		require.NoError(t, err)
		assert.True(t, res.DagsNotShipped)
		assert.Equal(t, []string{"no Dags were uploaded: there is no dags directory in " + dir + ", and the Deployment keeps the Dags it had"}, warnings)
	})
}

// A DAG upload that fails after the image was pushed says the image is
// deployed, which one, and the command that uploads the DAGs.
func TestDeployManifestImageDagUploadFailureNamesTheRetry(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: server.URL}).client(t)
	var warnings []string
	_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	var after *DagsAfterImageError
	require.ErrorAs(t, err, &after)
	assert.Equal(t, "registry.example.com/my-release/airflow:deploy-7", after.Image)
	assert.Contains(t, err.Error(), "the image registry.example.com/my-release/airflow:deploy-7 was pushed to Deployment dep")
	assert.Contains(t, err.Error(), "run astro deploy dep --dags")
}

// --image to an image Deployment is refused before the container engine is
// needed at all.
func TestDeployManifestImageRefusesImageOnlyBeforeTheEngine(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings)
	in.IncludeDags = false
	in.Engine = func() (string, []string, error) { return "", nil, errSomeContainerIssue }
	_, err := DeployManifestImage(client, in)
	require.ErrorIs(t, err, ErrDeploymentTypeIncorrectForImageOnly)
	assert.Empty(t, cmd.calls)
}

// Of several builds of a series, the newest is the base, whatever order the
// list gives them in.
func TestNewestForPicksTheNewestBuild(t *testing.T) {
	l := listedReleases{
		{Version: "3.1-3", AirflowVersion: "3.1.0"},
		{Version: "3.1-5", AirflowVersion: "3.1.1"},
		{Version: "3.1-4", AirflowVersion: "3.1.1"},
		{Version: "3.0-9", AirflowVersion: "3.0.6"},
		{Version: "12.1.0", AirflowVersion: "2.10.3"},
		{Version: "12.1.1", AirflowVersion: "2.10.3"},
	}
	assert.Equal(t, "3.1-5", l.newestFor("3.1"))
	assert.Equal(t, "3.1-3", l.newestFor("3.1.0"))
	assert.Equal(t, "12.1.1", l.newestFor("2.10"))
	assert.Equal(t, "", l.newestFor("3.2"))
}

// airflow2Only is what a platform lists for a Deployment without Airflow 3.
var airflow2Only = houston.RuntimeReleases{{Version: "12.1.1", AirflowVersion: "2.10.3"}}

// An Airflow 3 project to a Deployment the platform lists no Airflow 3
// runtime for is told Airflow 3 is not enabled for it: before the build when
// the pin says so, and on the built label when only the image can.
func TestDeployManifestImageRefusesAirflow3WhereItIsNotEnabled(t *testing.T) {
	t.Run("before the build", func(t *testing.T) {
		cmd := withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "12.1.1", listed: airflow2Only, version: "1.0.0"}).client(t)
		var warnings []string
		_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Airflow 3 is not enabled for this Deployment")
		assert.Contains(t, err.Error(), "pin Airflow 2")
		assert.False(t, cmd.called(" build "))
	})
	t.Run("on the built label", func(t *testing.T) {
		cmd := withManifestEngine(t, "3.1-5")
		client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "12.1.1", listed: airflow2Only, version: "1.0.0"}).client(t)
		dir := projectWithDags(t)
		// A FROM no reader can resolve: only the built image says its runtime.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("ARG BASE\nFROM ${BASE}\n"), 0o600))
		var warnings []string
		_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"}, &warnings))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot deploy Astro Runtime 3.1-5: Airflow 3 is not enabled for this Deployment")
		assert.True(t, cmd.called(" build "))
		assert.False(t, cmd.called(" push "))
	})
}

// A failed DAG upload after the push still is the upload's failure.
func TestDagsAfterImageErrorUnwraps(t *testing.T) {
	upload := errors.New("dag server said no")
	err := error(&DagsAfterImageError{Image: "img", DeploymentID: "dep", Err: upload})
	assert.ErrorIs(t, err, upload)
}

// The app config and the runtime list are asked for the workspace Houston
// places the Deployment in, not the one the deploy was given.
func TestDeployManifestImageAsksTheDeploymentsOwnWorkspace(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "2.1.0"}).client(t)
	client.ExpectedCalls = slices.DeleteFunc(client.ExpectedCalls, func(c *mock.Call) bool {
		return c.Method == "GetDeployment" || c.Method == "GetRuntimeReleases" || c.Method == "GetAppConfig"
	})
	client.On("GetDeployment", "dep").Return(&houston.Deployment{
		ID: "dep", ReleaseName: "my-release", ClusterID: "cl", RuntimeVersion: "3.1-5",
		Urls:          []houston.DeploymentURL{{Type: houston.RegistryURLType, URL: "registry.example.com"}},
		DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType}, DagDeploymentRead: true,
		Workspace: houston.Workspace{ID: "ws-real"},
	}, nil)
	client.On("GetAppConfig", houston.GetAppConfigRequest{ClusterID: "cl", WorkspaceUUID: "ws-real", DeploymentUUID: "dep"}).Return(&houston.AppConfig{Version: "2.1.0"}, nil).Once()
	client.On("GetRuntimeReleases", map[string]interface{}{"clusterId": "cl", "workspaceUuid": "ws-real", "deploymentUuid": "dep"}).Return(houstonLists, nil).Once()
	var warnings []string
	_, err := DeployManifestImage(client, manifestImageFor(t, &imagebuild.ManifestBuild{AirflowVersion: "3.1"}, &warnings))
	require.NoError(t, err)
	client.AssertExpectations(t)
}
