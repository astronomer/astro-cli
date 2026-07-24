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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// fakeBuilder is an ImageBuilder that records the request and never touches a
// daemon. It returns builtTag (the tag the caller asked to build, by default).
type fakeBuilder struct {
	mu      sync.Mutex
	gotReq  imagebuild.Request
	returns string
	err     error
}

func (f *fakeBuilder) Build(_ context.Context, req imagebuild.Request, _ localrt.Callbacks) (string, error) {
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
type fakeDocker struct {
	mu         sync.Mutex
	calls      [][]string
	inspectOut string
	failVerb   string
	failErr    error
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
	switch {
	case verb == "image" && len(args) > 1 && args[1] == "inspect":
		if s.Out != nil {
			io.WriteString(s.Out, f.inspectOut)
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
				AirflowVersion: "3.1",
				Packages:       []string{"libpq-dev"},
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
	req.Manifest.Astro.AirflowVersion = "2.9"
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
