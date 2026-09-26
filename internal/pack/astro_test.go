package pack

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// fakeBuilder is an ImageBuilder that records the request and never touches a
// daemon. It returns builtTag (the tag the caller asked to build, by default).
type fakeBuilder struct {
	mu      sync.Mutex
	gotReq  imagebuild.Request
	returns string
	err     error
}

func (f *fakeBuilder) BuildLocal(_ context.Context, req imagebuild.Request, _ localrt.Callbacks) (string, error) {
	f.mu.Lock()
	f.gotReq = req
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if f.returns != "" {
		return f.returns, nil
	}
	return req.Tag, nil
}

// fakeDocker is a Commander standing in for the docker CLI. It records each
// call, answers `image inspect` with a canned runtime label, and can fail a
// chosen verb. A `save` writes a small file so the size path has something to
// stat.
//
// With containerd set it models Docker's containerd image store on a host of
// another platform: only a tag the fake saw built (or tagged from a built one)
// reads its labels, and any other ref, such as a base pulled at a foreign
// platform, inspects with no labels, the way `docker image inspect` without
// --platform reads the host's missing variant of a multi-platform index.
type fakeDocker struct {
	mu         sync.Mutex
	calls      [][]string
	inspectOut string
	failVerb   string
	failErr    error
	containerd bool
	built      map[string]bool
}

func (f *fakeDocker) Run(_ context.Context, _ []string, s localrt.Stdio, name string, args ...string) error {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	f.mu.Unlock()
	verb := ""
	if len(args) > 0 {
		verb = args[0]
	}
	if f.failVerb != "" && verb == f.failVerb {
		if f.failErr != nil {
			return f.failErr
		}
		return errors.New("fake docker failure")
	}
	f.mu.Lock()
	if f.built == nil {
		f.built = map[string]bool{}
	}
	switch {
	case verb == "build" && len(args) > 2 && args[1] == "--tag":
		f.built[args[2]] = true
	case verb == "tag" && len(args) == 3:
		f.built[args[2]] = f.built[args[1]]
	}
	f.mu.Unlock()
	switch {
	case verb == "image" && len(args) > 1 && args[1] == "inspect":
		ref := args[len(args)-1]
		f.mu.Lock()
		readable := !f.containerd || f.built[ref]
		f.mu.Unlock()
		if s.Out != nil {
			if readable {
				io.WriteString(s.Out, f.inspectOut)
			} else {
				io.WriteString(s.Out, "<no value>\t<no value>")
			}
		}
	case verb == "save":
		// args: save --output <path> <ref>
		if len(args) >= 3 {
			os.WriteFile(args[2], []byte("tarball"), 0o600)
		}
	}
	return nil
}

func (f *fakeDocker) callStrings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}

func hasCall(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// testRequest is a package build over the etl-demo-shaped manifest, with a
// pinned WorkDir so the target does not make a temp dir.
func testRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		ProjectDir: t.TempDir(),
		WorkDir:    t.TempDir(),
		Platform:   "linux/amd64",
		Manifest: &manifest.Manifest{
			Project: manifest.Project{
				Name:         "my-project",
				Dependencies: []string{"apache-airflow==3.1.*", "pandas"},
			},
			Astro: manifest.Astro{
				Packages: []string{"libpq-dev"},
			},
		},
	}
}

func newAstro(builder ImageBuilder, docker imagebuild.Commander) *AstroTarget {
	return NewAstroTarget(builder, docker, "docker", nil)
}

func TestAstroBuildTagShape(t *testing.T) {
	builder := &fakeBuilder{}
	docker := &fakeDocker{inspectOut: "3.1-2\n"}
	res, err := newAstro(builder, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, TargetAstro, res.Target)
	assert.Equal(t, KindImage, res.Kind)
	assert.Equal(t, "3.1-2", res.RuntimeVersion)
	// astro-package/<name>:<runtime>-<7 hex>, runtime read off the label.
	assert.Regexp(t, regexp.MustCompile(`^astro-package/my-project:3\.1-2-[0-9a-f]{7}$`), res.Image)

	calls := docker.callStrings()
	assert.True(t, hasCall(calls, "version"), "probes the daemon: %v", calls)
	assert.True(t, hasCall(calls, "tag "+strings.Split(builder.gotReq.Tag, " ")[0]+" "+res.Image), "retags to final: %v", calls)
	assert.True(t, hasCall(calls, "astro-package/my-project:latest"), "adds the moving latest tag: %v", calls)
	// The builder started FROM the resolved runtime base and pinned the platform.
	assert.Equal(t, imagebuild.RuntimeImageRepo+":3.1", builder.gotReq.BaseImage)
	assert.Equal(t, "linux/amd64", builder.gotReq.Platform)
}

// The builder receives the manifest's build: the pin's base, its dependencies
// and its OS packages.
func TestAstroBuildHandsTheManifestBuildToTheBuilder(t *testing.T) {
	builder := &fakeBuilder{}
	_, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, imagebuild.RuntimeImageRepo+":3.1", builder.gotReq.BaseImage)
	assert.Equal(t, []string{"apache-airflow==3.1.*", "pandas"}, builder.gotReq.Dependencies)
	assert.Equal(t, []string{"libpq-dev"}, builder.gotReq.Packages)
	assert.Empty(t, builder.gotReq.Dockerfile)
}

// A [tool.astro] runtime is checked against the catalog before the build,
// its warnings ride on the result, and the image starts FROM that build.
func TestAstroBuildChecksAndBuildsFromTheRuntimeBuild(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Astro.Runtime = "3.1-12"
	var calls [][2]string
	req.CheckRuntime = func(_ context.Context, runtime, pin string) ([]runtimeversions.Finding, error) {
		calls = append(calls, [2]string{runtime, pin})
		return []runtimeversions.Finding{{Kind: runtimeversions.FindingYanked, Message: "runtime 3.1-12 is yanked"}}, nil
	}
	builder := &fakeBuilder{}
	res, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-12"}).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, [][2]string{{"3.1-12", "3.1"}}, calls)
	assert.Equal(t, []string{"runtime 3.1-12 is yanked"}, res.Warnings)
	assert.Equal(t, imagebuild.RuntimeImageRepo+":3.1-12", builder.gotReq.BaseImage)

	blocking := errors.New("another series")
	req.CheckRuntime = func(context.Context, string, string) ([]runtimeversions.Finding, error) { return nil, blocking }
	builder = &fakeBuilder{}
	_, err = newAstro(builder, &fakeDocker{inspectOut: "3.1-12"}).Build(context.Background(), req, localrt.Callbacks{})
	require.ErrorIs(t, err, blocking)
	assert.Empty(t, builder.gotReq.Tag, "nothing was built")
}

// With nothing to install the target still packages a single-platform image
// built at the requested platform, not the base's multi-platform tag: under
// the containerd store a pulled foreign-platform base inspects with no labels,
// so the runtime version would silently fall back to the manifest pin and the
// saved or pushed tag would be the index, not the one platform. Driven through
// the real imagebuild.Builder over the fake docker, since the build is the
// builder's and a fake builder would hide it.
func TestAstroBuildWithNothingToInstallPackagesASinglePlatformImage(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Project.Dependencies = []string{"apache-airflow==3.1.*"}
	req.Manifest.Astro.Packages = nil

	docker := &fakeDocker{inspectOut: "3.1-2", containerd: true}
	target := newAstro(imagebuild.New(docker, time.Now), docker)
	res, err := target.Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	calls := docker.callStrings()
	assert.Equal(t, "3.1-2", res.RuntimeVersion, "the label is read off the built image, got calls %v", calls)
	build, firstTag := -1, -1
	for i, c := range calls {
		if strings.HasPrefix(c, "docker build --tag astro-package/my-project:src-") && build < 0 {
			build = i
		}
		if strings.HasPrefix(c, "docker tag ") && firstTag < 0 {
			firstTag = i
		}
	}
	require.GreaterOrEqual(t, build, 0, "nothing to install still builds, got %v", calls)
	assert.Contains(t, calls[build], "--platform linux/amd64")
	require.GreaterOrEqual(t, firstTag, 0, "the image must be tagged, got %v", calls)
	assert.Less(t, build, firstTag, "the build must come before the tag, got %v", calls)
	assert.False(t, hasCall(calls, "docker pull"), "a pulled base is not what ships, got %v", calls)
	assert.False(t, hasCall(calls, "docker tag "+imagebuild.RuntimeImageRepo), "the base is never tagged as the artifact, got %v", calls)
}

func TestAstroBuildTagIsContentAddressed(t *testing.T) {
	docker := &fakeDocker{inspectOut: "3.1-2"}
	first, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	second, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, first.Image, second.Image, "same inputs produce the same tag")

	// A changed dependency moves the hash.
	req := testRequest(t)
	req.Manifest.Project.Dependencies = append(req.Manifest.Project.Dependencies, "requests")
	changed, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.NotEqual(t, first.Image, changed.Image, "a changed input moves the tag")
}

func TestAstroBuildTagOverrideSkipsLatest(t *testing.T) {
	docker := &fakeDocker{inspectOut: "3.1-2"}
	req := testRequest(t)
	req.Tag = "my-registry.example.com/team/proj:v1"
	res, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, res.Image)
	assert.False(t, hasCall(docker.callStrings(), ":latest"), "a --tag override owns naming, no latest")
}

func TestAstroBuildSaveWritesTarAndSize(t *testing.T) {
	docker := &fakeDocker{inspectOut: "3.1-2"}
	req := testRequest(t)
	req.Save = filepath.Join(t.TempDir(), "image.tar")
	res, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, req.Save, res.SavedPath)
	assert.Positive(t, res.Size, "reports the saved tarball size")
	assert.True(t, hasCall(docker.callStrings(), "save --output "+req.Save), "runs docker save: %v", docker.callStrings())
	_, statErr := os.Stat(req.Save)
	require.NoError(t, statErr)
}

func TestAstroBuildNoSaveOmitsPathAndSize(t *testing.T) {
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.SavedPath)
	assert.Zero(t, res.Size)
}

func TestAstroBuildNoDockerError(t *testing.T) {
	// The daemon probe fails, so the build stops with the plain no-Docker error
	// before it builds anything.
	docker := &fakeDocker{failVerb: "version"}
	_, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.ErrorIs(t, err, ErrNoDocker)
}

func TestAstroBuildFallsBackToManifestVersionWhenLabelMissing(t *testing.T) {
	// docker prints "<no value>" for an absent label; the tag falls back to the
	// manifest Airflow pin.
	docker := &fakeDocker{inspectOut: "<no value>"}
	res, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "3.1", res.RuntimeVersion)
	assert.Regexp(t, regexp.MustCompile(`^astro-package/my-project:3\.1-[0-9a-f]{7}$`), res.Image)
}

func TestAstroBuildFallsBackToAirflowLabel(t *testing.T) {
	// The runtime label is absent but the older airflow-version label is set;
	// the build reports it and tags a Docker-safe form of it (the '+' becomes
	// '-').
	docker := &fakeDocker{inspectOut: "\t3.1.8+astro.4"}
	res, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "3.1.8+astro.4", res.RuntimeVersion, "reports the raw label value")
	assert.Regexp(t, regexp.MustCompile(`^astro-package/my-project:3\.1\.8-astro\.4-[0-9a-f]{7}$`), res.Image)
}

func TestTagSafe(t *testing.T) {
	assert.Equal(t, "3.1-17", tagSafe("3.1-17"))
	assert.Equal(t, "3.1.8-astro.4", tagSafe("3.1.8+astro.4"))
	assert.Equal(t, "1.0", tagSafe("-1.0"), "a leading dash is dropped")
	assert.Equal(t, "unknown", tagSafe("+++"))
}

func TestAstroBuildRejectsNonAirflow3(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Project.Dependencies = []string{"apache-airflow==2.9.*", "pandas"}
	_, err := newAstro(&fakeBuilder{}, &fakeDocker{}).Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Airflow 3")
}

func TestAstroBuildRequiresProjectName(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Project.Name = ""
	_, err := newAstro(&fakeBuilder{}, &fakeDocker{}).Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestAstroBuildBuilderErrorSurfaces(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("install failed")}
	_, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install failed")
}

// TestAstroResultJSONShape pins the json object one reader handles for every
// target: target and kind always present, image fields for an image target,
// saved_path only with --save.
func TestAstroResultJSONShape(t *testing.T) {
	docker := &fakeDocker{inspectOut: "3.1-2"}
	req := testRequest(t)
	req.Save = filepath.Join(t.TempDir(), "image.tar")
	res, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	data, err := json.Marshal(res)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "astro", got["target"])
	assert.Equal(t, "image", got["kind"])
	assert.Equal(t, res.Image, got["image"])
	assert.Equal(t, "3.1-2", got["runtime_version"])
	assert.Equal(t, req.Save, got["saved_path"])
	// A tree/bundle target's fields stay absent for an image result.
	assert.NotContains(t, got, "tree_path")
	assert.NotContains(t, got, "bundle_path")
}

// declaringRequest is testRequest with a real declared Dockerfile on disk.
func declaringRequest(t *testing.T, rel, body string) Request {
	t.Helper()
	req := testRequest(t)
	req.Manifest.Astro.Dockerfile = rel
	full := filepath.Join(req.ProjectDir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	return req
}

// Packaging a project that declared its own Dockerfile builds THAT file.
//
// Before this the astro target resolved a runtime base from the pin and built a
// generated image, so `astro package` produced an artifact that did not contain
// the project's own build — every RUN and COPY silently absent, in the artifact
// whose whole purpose is to be the thing that ships.
func TestAstroBuildUsesADeclaredDockerfile(t *testing.T) {
	builder := &fakeBuilder{}
	req := declaringRequest(t, "docker/Dockerfile", "FROM my-own-base\nRUN echo hi\n")

	_, err := newAstro(builder, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(req.ProjectDir, "docker", "Dockerfile"), builder.gotReq.Dockerfile)
	assert.Equal(t, req.ProjectDir, builder.gotReq.Context,
		"the context has to be the project, or the file's own COPY paths do not resolve")
	assert.Empty(t, builder.gotReq.BaseImage,
		"a declared Dockerfile names its own FROM, so resolving a base would be a version-service call for an image nobody uses")
}

// The content address covers the Dockerfile's BYTES, not just its path.
//
// In Dockerfile mode the file is the whole build and imagebuild ignores base,
// dependencies and packages — so a hash built only from those gave two different
// images the same content-addressed tag. Editing the Dockerfile then republished
// under the tag the previous image already held, which is the one thing a
// content-addressed scheme exists to prevent.
func TestAstroBuildContentAddressCoversTheDockerfileBody(t *testing.T) {
	build := func(t *testing.T, body string) string {
		t.Helper()
		req := declaringRequest(t, "Dockerfile", body)
		res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err)
		return res.Image
	}

	same := build(t, "FROM my-own-base\nRUN echo hi\n")
	again := build(t, "FROM my-own-base\nRUN echo hi\n")
	assert.Equal(t, same, again, "identical inputs must produce the same tag")

	edited := build(t, "FROM my-own-base\nRUN echo something-else\n")
	assert.NotEqual(t, same, edited,
		"an edited Dockerfile is a different image and must not reuse the tag")
}

// A declaration naming nothing fails before the build, naming the path.
//
// contentHash reads the file, so this is refused here rather than reaching
// imagebuild — either way the message names the declared path, which is the
// thing to fix.
func TestAstroBuildRefusesAnUnreadableDeclaration(t *testing.T) {
	req := testRequest(t)
	req.Manifest.Astro.Dockerfile = "docker/Dockerfile" // never written

	_, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2"}).Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker/Dockerfile")
}

// A declared Dockerfile on a non-Astro base is REPORTED, not refused.
//
// The gap was silence: package tagged the image with the manifest pin, asserting
// an Astro Runtime version the image does not have, and cloud/deploy then refused
// it on the label. A hard error here closed the silence and closed a legitimate
// workflow with it — a Dockerfile on a plain python base, packaged with --tag and
// --save for a self-hosted Airflow, which worked before and which the `oss` target
// that should serve it is still a stub for. So this warns and carries on.
func TestAstroBuildWarnsWhenADeclaredBuildHasNoRuntimeLabel(t *testing.T) {
	req := declaringRequest(t, "Dockerfile", "FROM python:3.12-slim\nRUN echo hi\n")
	var lines []string
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l.Text) }}

	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: ""}).Build(context.Background(), req, cb)
	require.NoError(t, err, "a self-hosted image is a real thing to package")
	assert.Contains(t, strings.Join(lines, "\n"), "not based on Astro Runtime",
		"the artifact cannot be deployed to Astro and the user has to hear that here, not at deploy time")
	assert.NotEmpty(t, res.Image)
}

// The warning is about the RUNTIME label specifically, which is the one deploy
// reads. readVersionLabels also surfaces the older airflow-version label, so a
// `FROM astronomerinc/ap-airflow:...-onbuild` image satisfies it and would have
// passed a check built on that helper — then been refused by deploy anyway.
func TestAstroBuildWarnsForAnAirflowLabelledImageToo(t *testing.T) {
	req := declaringRequest(t, "Dockerfile", "FROM astronomerinc/ap-airflow:2.7.1-onbuild\n")
	var lines []string
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l.Text) }}

	// One inspect returns both labels, tab-separated: no runtime label, and the
	// airflow label answering in its place.
	docker := &fakeDocker{inspectOut: "<no value>\t2.7.1"}
	_, err := newAstro(&fakeBuilder{}, docker).Build(context.Background(), req, cb)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(lines, "\n"), "not based on Astro Runtime",
		"deploy reads only the runtime label, so this has to warn on the same test deploy applies")
}

// A generated build never warns: its base is an Astro runtime by construction.
func TestAstroBuildDoesNotWarnForAGeneratedImage(t *testing.T) {
	var lines []string
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l.Text) }}
	res, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: ""}).Build(context.Background(), testRequest(t), cb)
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(lines, "\n"), "not based on Astro Runtime")
	assert.Contains(t, res.Image, "3.1", "the manifest pin names the tag when the label is absent")
}
