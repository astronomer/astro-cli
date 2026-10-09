package deploy

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// projectStepCmd is fakeImageCmd that also records the step that copies the
// project into the image: its context, and the ignore file beside its
// Dockerfile, read while it still exists (the deploy removes its build
// directory once the build returns).
type projectStepCmd struct {
	fakeImageCmd
	projectContext string
	ignore         []string
}

func (f *projectStepCmd) Run(ctx context.Context, env []string, s localrt.Stdio, name string, args ...string) error {
	if len(args) > 0 && (args[0] == "build" || args[0] == "buildx") {
		for i, a := range args {
			if a == "--file" && i+1 < len(args) && strings.HasSuffix(args[i+1], "Dockerfile.astro-project") {
				f.projectContext = args[len(args)-1]
				data, err := os.ReadFile(args[i+1] + ".dockerignore")
				if err == nil {
					f.ignore = strings.Split(strings.TrimSpace(string(data)), "\n")
				}
			}
		}
	}
	return f.fakeImageCmd.Run(ctx, env, s, name, args...)
}

// withProjectStep replaces withImageSeams' build commander with one that
// records the project step.
func withProjectStep(t *testing.T) *projectStepCmd {
	t.Helper()
	withImageSeams(t, "3.1-2")
	cmd := &projectStepCmd{}
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	return cmd
}

// projectWithCode is a project with a DAG, a plugin, an include file and a
// top-level package.
func projectWithCode(t *testing.T) string {
	t.Helper()
	dir := manifestProjectDir(t)
	for name, body := range map[string]string{"plugins/x.py": "X = 1\n", "include/y.sql": "select 1;\n", "utils/u.py": "U = 1\n"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

// mockDeploymentWith stubs the Deployment lookup with these DAG settings over
// the usual fixture.
func mockDeploymentWith(client *astrov1_mocks.ClientWithResponsesInterface, dagDeploy, remoteExecution bool) {
	standard := astrov1.DeploymentTypeSTANDARD
	dep := &astrov1.Deployment{
		Id:                  "test-deployment-id",
		Name:                "test-deployment",
		OrganizationId:      "test-org-id",
		WorkspaceId:         "test-ws-id",
		AstroRuntimeVersion: "3.1-2",
		Type:                &standard,
		IsDagDeployEnabled:  dagDeploy,
	}
	if remoteExecution {
		dep.RemoteExecution = &astrov1.DeploymentRemoteExecution{Enabled: true}
	}
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      dep,
	}, nil)
}

// countUploads replaces the DAG uploader for one test and counts its calls.
func countUploads(t *testing.T) *int {
	t.Helper()
	orig := azureUploader
	n := 0
	azureUploader = func(string, io.Reader) (string, error) { n++; return "tarball-v1", nil }
	t.Cleanup(func() { azureUploader = orig })
	return &n
}

func manifestBuildOf(dir string) imagebuild.ManifestBuild {
	return imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dependencies: []string{"pandas"}}
}

// deployWith runs a deploy against a Deployment with these DAG settings and
// the create/finalize calls stubbed.
func deployWith(t *testing.T, dagDeploy, remote bool, in *ManifestImageDeployInput) (ManifestImageDeployResult, *astrov1_mocks.ClientWithResponsesInterface, error) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, dagDeploy, remote)
	mockDeploymentOptions(client, "3.1-2")
	upload := ""
	if dagDeploy {
		upload = "https://upload-url"
	}
	mockCreateImageDeploy(client, upload)
	mockFinalizeDeploy(client)
	in.DeploymentID = "test-deployment-id"
	res, err := DeployManifestImage(*in, client)
	return res, client, err
}

// A Deployment that takes DAG deploys gets the project in the image, without
// dags/, and its DAGs as the upload: the 1.x path's build without dags.
func TestDeployManifestImage_DagDeployShipsTheProjectWithoutDagsAndUploadsThem(t *testing.T) {
	cmd := withProjectStep(t)
	uploads := countUploads(t)
	dir := projectWithCode(t)

	res, _, err := deployWith(t, true, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
	require.NoError(t, err)

	assert.Equal(t, dir, cmd.projectContext, "the project is the build context")
	assert.Equal(t, "dags", cmd.ignore[len(cmd.ignore)-1])
	assert.Equal(t, 1, *uploads)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	assert.Equal(t, manifestdeploy.DagsUploaded, res.Dags)
}

// A Deployment that takes no DAG deploys runs the image's DAGs, so a "both"
// deploy builds dags/ in and uploads nothing, as the 1.x path does, rather
// than refusing.
func TestDeployManifestImage_NoDagDeployBuildsDagsIntoTheImage(t *testing.T) {
	cmd := withProjectStep(t)
	uploads := countUploads(t)
	dir := projectWithCode(t)

	res, client, err := deployWith(t, false, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
	require.NoError(t, err)

	assert.Equal(t, dir, cmd.projectContext)
	assert.NotContains(t, cmd.ignore, "dags")
	assert.Zero(t, *uploads, "nothing is uploaded to a Deployment that takes no DAG deploys")
	assert.Empty(t, res.DagTarballVersion)
	assert.Equal(t, manifestdeploy.DagsBuiltIn, res.Dags)
	client.AssertCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(r astrov1.CreateDeployRequest) bool {
		return r.Type == astrov1.CreateDeployRequestTypeIMAGEANDDAG
	}))
	client.AssertCalled(t, "FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(r astrov1.FinalizeDeployRequest) bool {
		return r.DagTarballVersion == nil
	}))
}

// Ignore rules that leave every DAG file out would ship an image with no DAGs
// to a Deployment that runs only the image's. 1.x removed a "dags/" line
// before the build; this refuses, for any rule that drops them all, and edits
// nothing.
func TestDeployManifestImage_RefusesToBuildDagsInWhenTheIgnoreFileLeavesThemOut(t *testing.T) {
	for _, rule := range []string{"dags/", "dags", "dags/**", "dags/*", "**/*.py", "*"} {
		t.Run(rule, func(t *testing.T) {
			cmd := withProjectStep(t)
			dir := projectWithCode(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(rule+"\n"), 0o600))

			_, _, err := deployWith(t, false, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no DAG file in dags/ would reach the image")
			assert.False(t, hasImageCall(cmd.calls, " --tag "), "refused before the build, got %v", cmd.calls)
		})
	}
	for name, build := range map[string]func(dir string) imagebuild.ManifestBuild{
		"generated": manifestBuildOf,
	} {
		t.Run(name, func(t *testing.T) {
			cmd := withProjectStep(t)
			dir := projectWithCode(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))

			_, client, err := deployWith(t, false, false, &ManifestImageDeployInput{Build: build(dir), IncludeDags: true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no DAG file in dags/ would reach the image")
			assert.False(t, hasImageCall(cmd.calls, " --tag "), "refused before the build, got %v", cmd.calls)
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			got, err := os.ReadFile(filepath.Join(dir, ".dockerignore"))
			require.NoError(t, err)
			assert.Equal(t, "dags/\n", string(got), "the project's file is not edited")
		})
	}
}

// With DAG deploys on, the DAGs are left out of the image anyway, so the same
// rule is no reason to refuse.
func TestDeployManifestImage_DagsInTheIgnoreFileAreFineWithDagDeploy(t *testing.T) {
	withProjectStep(t)
	countUploads(t)
	dir := projectWithCode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))

	_, _, err := deployWith(t, true, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
	require.NoError(t, err)
}

// --image leaves the running DAGs in place, which a Deployment whose DAGs are
// in its image cannot do. The 1.x path refuses it, before anything is built.
func TestDeployManifestImage_ImageOnlyIsRefusedWithoutDagDeploy(t *testing.T) {
	for name, imageName := range map[string]string{"built": "", "prebuilt": "astro-package/demo:latest"} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockDeploymentWith(client, false, false)
			cmd := withProjectStep(t)

			_, err := DeployManifestImage(ManifestImageDeployInput{
				Build:        manifestBuildOf(projectWithCode(t)),
				DeploymentID: "test-deployment-id",
				ImageName:    imageName,
			}, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "without --image")
			assert.Contains(t, err.Error(), "astro deployment update test-deployment-id --dag-deploy enable")
			assert.False(t, hasImageCall(cmd.calls, " --tag "), "refused before the build, got %v", cmd.calls)
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// With DAG deploys on, --image keeps DAGs out of the image and uploads none.
func TestDeployManifestImage_ImageOnlyWithDagDeployShipsNoDags(t *testing.T) {
	cmd := withProjectStep(t)
	uploads := countUploads(t)

	res, _, err := deployWith(t, true, false, &ManifestImageDeployInput{Build: manifestBuildOf(projectWithCode(t))})
	require.NoError(t, err)

	assert.Equal(t, "dags", cmd.ignore[len(cmd.ignore)-1])
	assert.Zero(t, *uploads)
	assert.Empty(t, res.Dags)
}

// Remote execution runs the DAGs elsewhere. As on the 1.x path, the image
// carries none and none are uploaded, whether DAG deploys are on or off, and
// --image is not refused.
func TestDeployManifestImage_RemoteExecutionShipsNoDags(t *testing.T) {
	for _, dagDeploy := range []bool{true, false} {
		for _, both := range []bool{true, false} {
			t.Run(map[bool]string{true: "dag deploy on", false: "dag deploy off"}[dagDeploy]+map[bool]string{true: ", both", false: ", --image"}[both], func(t *testing.T) {
				cmd := withProjectStep(t)
				uploads := countUploads(t)

				res, _, err := deployWith(t, dagDeploy, true, &ManifestImageDeployInput{Build: manifestBuildOf(projectWithCode(t)), IncludeDags: both})
				require.NoError(t, err)

				assert.Equal(t, "dags", cmd.ignore[len(cmd.ignore)-1])
				assert.Zero(t, *uploads)
				if both {
					assert.Equal(t, manifestdeploy.DagsNone, res.Dags)
				}
			})
		}
	}
}

// A prebuilt image to a Deployment without DAG deploys ships as it is, with
// whatever DAGs it carries, and nothing is uploaded; the result says the CLI
// did not put them there.
func TestDeployManifestImage_PrebuiltImageWithoutDagDeploy(t *testing.T) {
	cmd := withProjectStep(t)
	uploads := countUploads(t)

	res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
		Build:       imagebuild.ManifestBuild{ProjectDir: projectWithCode(t)},
		ImageName:   "astro-package/demo:latest",
		IncludeDags: true,
	})
	require.NoError(t, err)
	assert.False(t, hasImageCall(cmd.calls, " --tag "), "a prebuilt image is not built, got %v", cmd.calls)
	assert.Zero(t, *uploads)
	assert.Equal(t, manifestdeploy.DagsFromImage, res.Dags)
}

// A declared Dockerfile's context is the project already: there is no project
// step, and its own COPY lines decide what DAGs the image carries.
func TestDeployManifestImage_DeclaredDockerfileWithoutDagDeploy(t *testing.T) {
	cmd := withProjectStep(t)
	countUploads(t)
	dir := projectWithCode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))

	res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
		Build:       imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
		IncludeDags: true,
	})
	require.NoError(t, err)
	assert.Empty(t, cmd.projectContext, "no project step")
	var builds []string
	for _, c := range cmd.calls {
		if strings.HasPrefix(c, "docker build") {
			builds = append(builds, c)
		}
	}
	require.Len(t, builds, 1)
	assert.True(t, strings.HasSuffix(builds[0], " "+dir), builds[0])
	assert.Equal(t, manifestdeploy.DagsFromImage, res.Dags)
}

// A rule that leaves out only some DAG files, or other files in dags/, is not
// a reason to refuse.
func TestDeployManifestImage_SomeDagsLeftOutIsFine(t *testing.T) {
	withProjectStep(t)
	dir := projectWithCode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "other.py"), []byte("# dag\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/example.py\n"), 0o600))

	res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
	require.NoError(t, err)
	assert.Equal(t, manifestdeploy.DagsBuiltIn, res.Dags)
}

// With no DAG files to build in, the deploy goes ahead, as 1.x did, and says
// the Deployment will run none rather than claiming DAGs are in the image.
func TestDeployManifestImage_NoDagFilesToBuildIn(t *testing.T) {
	for name, prep := range map[string]func(dir string){
		"no dags directory": func(dir string) { require.NoError(t, os.RemoveAll(filepath.Join(dir, "dags"))) },
		"no .py files":      func(dir string) { require.NoError(t, os.Remove(filepath.Join(dir, "dags", "example.py"))) },
	} {
		t.Run(name, func(t *testing.T) {
			withProjectStep(t)
			dir := projectWithCode(t)
			prep(dir)
			var warnings []string

			res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
				Build: manifestBuildOf(dir), IncludeDags: true,
				Warn: func(w string) { warnings = append(warnings, w) },
			})
			require.NoError(t, err)
			assert.Equal(t, manifestdeploy.DagsEmpty, res.Dags)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], "dags/ holds no DAG files")
		})
	}
}

// A prebuilt image or a declared Dockerfile to a Deployment without DAG
// deploys is all the Deployment will run, and the CLI did not put DAGs in it,
// so the deploy says so. 1.x deployed both silently.
func TestDeployManifestImage_WarnsThatOnlyTheImagesDagsWillRun(t *testing.T) {
	t.Run("prebuilt", func(t *testing.T) {
		withProjectStep(t)
		var warnings []string
		res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
			Build:     imagebuild.ManifestBuild{ProjectDir: projectWithCode(t)},
			ImageName: "astro-package/demo:latest", IncludeDags: true,
			Warn: func(w string) { warnings = append(warnings, w) },
		})
		require.NoError(t, err)
		assert.Equal(t, manifestdeploy.DagsFromImage, res.Dags)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "runs only the DAGs inside astro-package/demo:latest")
	})
	t.Run("declared Dockerfile", func(t *testing.T) {
		withProjectStep(t)
		dir := projectWithCode(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))
		var warnings []string
		res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
			Build:       imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
			IncludeDags: true,
			Warn:        func(w string) { warnings = append(warnings, w) },
		})
		require.NoError(t, err)
		assert.Equal(t, manifestdeploy.DagsFromImage, res.Dags)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "runs only the DAGs that Dockerfile copies into the image")
	})
}

// A declared Dockerfile whose ignore rules leave dags/ out may make its DAGs
// itself, so that is a warning, not a refusal; its own
// <Dockerfile>.dockerignore is the file read, when there is one.
func TestDeployManifestImage_DeclaredDockerfileLeavingDagsOutWarns(t *testing.T) {
	for name, file := range map[string]string{"its own ignore file": "Dockerfile.dockerignore", ".dockerignore": ".dockerignore"} {
		t.Run(name, func(t *testing.T) {
			withProjectStep(t)
			dir := projectWithCode(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte("dags\n"), 0o600))
			var warnings []string

			res, _, err := deployWith(t, false, false, &ManifestImageDeployInput{
				Build:       imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
				IncludeDags: true,
				Warn:        func(w string) { warnings = append(warnings, w) },
			})
			require.NoError(t, err)
			assert.Equal(t, manifestdeploy.DagsFromImage, res.Dags)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], "leaves out every DAG file in dags/, so unless the Dockerfile makes them")
		})
	}
}

// Without buildx, a deploy whose DAGs are not built in falls back to the
// dependency-only image it built before, and says the project is not in it.
func TestDeployManifestImage_WithoutBuildxShipsTheDependenciesAndWarns(t *testing.T) {
	cmd := withProjectStep(t)
	cmd.noBuildx = true
	countUploads(t)
	var warnings []string

	_, _, err := deployWith(t, true, false, &ManifestImageDeployInput{
		Build: manifestBuildOf(projectWithCode(t)), IncludeDags: true,
		Warn: func(w string) { warnings = append(warnings, w) },
	})
	require.NoError(t, err)
	assert.Empty(t, cmd.projectContext, "no step copies the project")
	assert.False(t, hasImageCall(cmd.calls, "buildx build"), "the legacy dependency build: %v", cmd.calls)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "are NOT in it")
	assert.Contains(t, warnings[0], "buildx")
}

// Where the DAGs have to be built in, a dependency-only image would leave the
// Deployment with none, so without buildx the deploy is refused.
func TestDeployManifestImage_WithoutBuildxRefusesToBuildDagsIn(t *testing.T) {
	cmd := withProjectStep(t)
	cmd.noBuildx = true

	_, client, err := deployWith(t, false, false, &ManifestImageDeployInput{Build: manifestBuildOf(projectWithCode(t)), IncludeDags: true})
	require.ErrorIs(t, err, imagebuild.ErrNoProjectBuilder)
	assert.Contains(t, err.Error(), "its DAGs have to be built into the image")
	assert.False(t, hasImageCall(cmd.calls, " --tag "), "refused before the build, got %v", cmd.calls)
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The local image tag is this deploy's alone, names what it carries of the
// DAGs, and is dropped once pushed, so two deploys from one checkout never
// share one.
func TestDeployManifestImage_TagsEachBuildUniquely(t *testing.T) {
	dir := projectWithCode(t)
	tags := map[string]bool{}
	for _, dagDeploy := range []bool{true, true, false} {
		cmd := withProjectStep(t)
		countUploads(t)
		_, _, err := deployWith(t, dagDeploy, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
		require.NoError(t, err)
		tag := builtTag(t, cmd.calls, deployImageTag(dir))
		mode := map[bool]string{true: ":nodags-", false: ":dags-"}[dagDeploy]
		assert.Contains(t, tag, mode)
		assert.Contains(t, cmd.calls, "docker image rm "+tag, "the pushed image is removed, with the dependency image under it")
		tags[tag] = true
	}
	assert.Len(t, tags, 3, "every build has a tag of its own")
}

// Files git ignores that the image will carry are named in a warning, and a
// project that is not a git repository gets none.
func TestDeployManifestImage_WarnsAboutGitignoredFilesItWillCarry(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := projectWithCode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("notes.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("n"), 0o600))

	deploy := func() []string {
		t.Helper()
		withProjectStep(t)
		countUploads(t)
		var warnings []string
		_, _, err := deployWith(t, true, false, &ManifestImageDeployInput{
			Build: manifestBuildOf(dir), IncludeDags: true,
			Warn: func(w string) { warnings = append(warnings, w) },
		})
		require.NoError(t, err)
		return warnings
	}
	assert.Empty(t, deploy(), "not a git repository")

	git := exec.Command("git", "-C", dir, "init", "-q")
	git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	require.NoError(t, git.Run())
	warnings := deploy()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "notes.txt")
}

// Gitignored files that look like credentials would be baked into an image
// pushed to a registry, so the deploy is refused before it builds, naming
// them; other gitignored files (a dbt target/, say) ship, with the warning.
func TestDeployManifestImage_RefusesGitignoredSecrets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := projectWithCode(t)
	for name, body := range map[string]string{".gitignore": "target/\nsecrets/\n", "target/manifest.json": "{}", "secrets/prod.pem": "k"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	git := exec.Command("git", "-C", dir, "init", "-q")
	git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	require.NoError(t, git.Run())

	cmd := withProjectStep(t)
	_, client, err := deployWith(t, true, false, &ManifestImageDeployInput{Build: manifestBuildOf(dir), IncludeDags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets/prod.pem")
	assert.NotContains(t, err.Error(), "target/manifest.json")
	assert.False(t, hasImageCall(cmd.calls, " --tag "), "refused before the build, got %v", cmd.calls)
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("secrets/\n"), 0o600))
	withProjectStep(t)
	countUploads(t)
	var warnings []string
	_, _, err = deployWith(t, true, false, &ManifestImageDeployInput{
		Build: manifestBuildOf(dir), IncludeDags: true,
		Warn: func(w string) { warnings = append(warnings, w) },
	})
	require.NoError(t, err, "left out by .dockerignore, the key no longer ships")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "target/manifest.json")
}

// The engine is asked once per deploy: the answer is handed to the build.
func TestDeployManifestImage_AsksTheEngineOnce(t *testing.T) {
	cmd := withProjectStep(t)
	countUploads(t)
	_, _, err := deployWith(t, true, false, &ManifestImageDeployInput{Build: manifestBuildOf(projectWithCode(t)), IncludeDags: true})
	require.NoError(t, err)
	n := 0
	for _, c := range cmd.calls {
		if strings.Contains(c, "type=local,dest=") {
			n++
		}
	}
	assert.Equal(t, 1, n, "one check build: %v", cmd.calls)
}

func TestRepositoryName(t *testing.T) {
	for name, want := range map[string]string{
		"MyProject":       "myproject",
		"my_project.v2":   "my-project-v2",
		"--Odd  Name!!--": "odd-name",
		"日本":              "project",
		"":                "project",
		"a":               "a",
	} {
		assert.Equal(t, want, repositoryName(name), name)
	}
	assert.True(t, strings.HasPrefix(deployImageTag(filepath.Join(t.TempDir(), "My Project")), "astro-deploy/my-project"))
}
