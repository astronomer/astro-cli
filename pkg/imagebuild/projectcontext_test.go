package imagebuild

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func projectRequest(t *testing.T, project string) Request {
	t.Helper()
	return Request{
		WorkDir:        t.TempDir(),
		BaseImage:      "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:            "astro-deploy/p-abc123",
		ProjectContext: project,
		Platform:       "linux/amd64",
		Secrets:        []string{"id=netrc,env=NETRC"},
		Bin:            "docker",
	}
}

// engine is a fakeCmd hook answering the probes as an engine would: what
// `--version` prints, whether `buildx version` answers, the help of `build`,
// and, for Docker, a current context "orbstack" whose builder has the docker
// driver; for podman, a local service. fail, when set, fails the calls it
// matches, probes included.
func engine(version string, buildx bool, buildHelp string, fail func(call string) error) func(string, rt.Stdio) error {
	return engineWith(version, buildx, buildHelp, "docker", "honors", fail)
}

// engineWith is engine with the builder's driver chosen, and what the
// ignore-file check build does: "honors" the ignore file, "ignores" it
// (copies the left-out file), or "fails".
func engineWith(version string, buildx bool, buildHelp, driver, check string, fail func(call string) error) func(string, rt.Stdio) error {
	return func(call string, s rt.Stdio) error {
		if fail != nil {
			if err := fail(call); err != nil {
				return err
			}
		}
		_, args, _ := strings.Cut(call, " ")
		switch args {
		case "--version":
			_, _ = io.WriteString(s.Out, version)
		case "buildx version":
			if !buildx {
				return errors.New("docker: 'buildx' is not a docker command")
			}
		case "build --help":
			_, _ = io.WriteString(s.Out, buildHelp)
		case "context show":
			_, _ = io.WriteString(s.Out, "orbstack\n")
		case "buildx inspect orbstack":
			_, _ = io.WriteString(s.Out, "Name:          orbstack\nDriver:        "+driver+"\nLast Activity: now\n")
		}
		if _, dest, ok := strings.Cut(call, "--output type=local,dest="); ok {
			return checkBuild(strings.Fields(dest)[0], check)
		}
		return nil
	}
}

const dockerVersion = "Docker version 29.4.0, build 1234567"

// checkBuild plays the ignore-file check build's export into dest: "honors"
// exports the kept files only, "ignores" the markers too, "rootonly" the
// marker in the subdirectory (a "**/" rule matching only at the root),
// "nosub" leaves out the subdirectory's kept file, "empty" exports nothing,
// and "fails" exports the kept files and then fails.
func checkBuild(dest, check string) error {
	if err := os.MkdirAll(filepath.Join(dest, probeSub), 0o700); err != nil {
		return err
	}
	if check == "empty" {
		return nil
	}
	write := func(rel string) error { return os.WriteFile(filepath.Join(dest, rel), nil, 0o600) }
	if err := write(probeKeep); err != nil {
		return err
	}
	if check != "nosub" {
		if err := write(filepath.Join(probeSub, probeKeep)); err != nil {
			return err
		}
	}
	switch check {
	case "ignores":
		if err := write(probeMarker); err != nil {
			return err
		}
		return write(filepath.Join(probeSub, probeMarker))
	case "rootonly":
		return write(filepath.Join(probeSub, probeMarker))
	case "fails":
		return errors.New("exit status 1")
	}
	return nil
}

// callsContaining are the calls containing sub. The ignore-file check build
// is left out unless sub asks for it, so a count of builds counts the image
// builds.
func callsContaining(calls []string, sub string) []string {
	var out []string
	for _, c := range calls {
		if strings.Contains(c, "--output type=local") && !strings.Contains(sub, "--output") {
			continue
		}
		if strings.Contains(c, sub) {
			out = append(out, c)
		}
	}
	return out
}

// A ProjectContext build installs the dependencies over the base, as every
// generated build does, then copies the project in with BuildKit (buildx),
// the project as the context, and drops the intermediate tag.
func TestBuildShipsTheProjectAsTheContextOfASecondStep(t *testing.T) {
	project := t.TempDir()
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	req := projectRequest(t, project)

	got, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p-abc123", got)

	assert.Equal(t, []string{"docker --version", "docker buildx version", "docker context show", "docker buildx inspect orbstack"}, cmd.calls[:4],
		"the engine is asked before anything is built")
	assert.Contains(t, cmd.calls[4], "docker buildx build --builder orbstack --file ")
	assert.Contains(t, cmd.calls[4], "--output type=local,dest=", "then a check build proves the ignore file is read")
	deps := callsContaining(cmd.calls, "--tag astro-deploy/p-abc123:latest-deps ")
	require.Len(t, deps, 1, "%v", cmd.calls)
	assert.True(t, strings.HasPrefix(deps[0], "docker buildx build --builder orbstack --load --tag astro-deploy/p-abc123:latest-deps "), deps[0])
	assert.Contains(t, deps[0], "--pull")
	assert.Contains(t, deps[0], "--secret id=netrc,env=NETRC")
	assert.True(t, strings.HasSuffix(deps[0], " "+filepath.Join(req.WorkDir, buildContextDir)), deps[0])

	projectDF := filepath.Join(req.WorkDir, projectDockerfileName)
	proj := callsContaining(cmd.calls, "buildx build")
	require.Len(t, proj, 2, "%v", cmd.calls)
	assert.Equal(t, "docker buildx build --builder orbstack --load --tag astro-deploy/p-abc123 --file "+projectDF+" --platform linux/amd64 "+project, proj[1],
		"BuildKit on the current context's docker-driver builder, no --pull (the base is local), no secrets (nothing runs), the project as the context")
	df, err := os.ReadFile(projectDF)
	require.NoError(t, err)
	assert.Equal(t, "FROM astro-deploy/p-abc123:latest-deps\nCOPY --chown=astro:0 . .\n", string(df))
	_, err = os.Stat(projectDF + ".dockerignore")
	require.NoError(t, err, "the ignore file sits beside the Dockerfile, where BuildKit reads it")

	assert.Equal(t, "docker image rm --no-prune astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
}

// The step that copies the project runs with DOCKER_BUILDKIT=1 in its
// environment, as well as through buildx.
func TestBuildProjectStepAsksForBuildKit(t *testing.T) {
	var env []string
	cmd := &envCmd{fakeCmd: fakeCmd{run: engine(dockerVersion, true, "", nil)}, onBuildx: func(e []string) { env = e }}
	_, err := New(cmd, func() time.Time { return fixedTime }).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.NoError(t, err)
	assert.Contains(t, env, "DOCKER_BUILDKIT=1")
}

// envCmd is fakeCmd that hands the environment of a buildx call to onBuildx.
type envCmd struct {
	fakeCmd
	onBuildx func([]string)
}

func (e *envCmd) Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	if len(args) > 0 && args[0] == "buildx" && len(args) > 1 && args[1] == "build" {
		e.onBuildx(env)
	}
	return e.fakeCmd.Run(ctx, env, s, name, args...)
}

// Without buildx, `docker build` may be the legacy builder, which ignores
// <Dockerfile>.dockerignore and would copy .env and the rest into the image.
// The build is refused before anything is built.
func TestBuildProjectRefusesDockerWithoutBuildx(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, false, "", nil)}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.ErrorIs(t, err, ErrNoProjectBuilder)
	assert.Contains(t, err.Error(), "buildx")
	assert.Empty(t, callsContaining(cmd.calls, "--tag"), "nothing is built: %v", cmd.calls)
}

func TestBuildWithAProjectContextSkipsTheFastPath(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	req := projectRequest(t, t.TempDir())
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, got)
	assert.Len(t, callsContaining(cmd.calls, "build --builder"), 2)
}

// Without a ProjectContext the build is what it always was: one build over
// the two dependency files, no probes, and the fast path when there is
// nothing to install. Local Docker mode and Astro Desktop build this way.
func TestBuildWithoutAProjectContextIsUnchanged(t *testing.T) {
	req := projectRequest(t, "")
	cmd := &fakeCmd{}
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.BaseImage, got)
	assert.Empty(t, cmd.calls)

	req.Dependencies = []string{"pandas"}
	_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], "docker build --tag astro-deploy/p-abc123 ")
	_, err = os.Stat(filepath.Join(req.WorkDir, projectDockerfileName))
	assert.True(t, os.IsNotExist(err), "no project step")
}

// Podman, called by its name or through the podman-docker shim, is told the
// ignore file with --ignorefile, and drops the intermediate name with untag.
func TestBuildWithAProjectContextOnPodman(t *testing.T) {
	for name, bin := range map[string]string{"podman": "/opt/podman/bin/podman", "the podman-docker shim": "docker"} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			cmd := &fakeCmd{run: engine("podman version 5.8.2", false, "      --ignorefile string   path to an alternate .dockerignore file", nil)}
			req := projectRequest(t, project)
			req.Bin = bin

			_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
			require.NoError(t, err)

			ignore := filepath.Join(req.WorkDir, projectDockerfileName) + ".dockerignore"
			proj := callsContaining(cmd.calls, "--ignorefile")
			require.Len(t, proj, 1, "%v", cmd.calls)
			assert.True(t, strings.HasPrefix(proj[0], bin+" build --tag astro-deploy/p-abc123 --file "), proj[0])
			assert.Contains(t, proj[0], "--ignorefile "+ignore)
			assert.Empty(t, callsContaining(cmd.calls, "buildx"))
			assert.Equal(t, bin+" untag astro-deploy/p-abc123:latest-deps astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
		})
	}
}

// A podman whose build has no --ignorefile cannot be told the ignore file, so
// the build is refused before anything is built.
func TestBuildProjectRefusesPodmanWithoutIgnorefile(t *testing.T) {
	cmd := &fakeCmd{run: engine("podman version 2.0.0", false, "      --file string   Dockerfile", nil)}
	req := projectRequest(t, t.TempDir())
	req.Bin = "podman"
	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, ErrNoProjectBuilder)
	assert.Contains(t, err.Error(), "--ignorefile")
	assert.Empty(t, callsContaining(cmd.calls, "build --tag"))
}

func TestBuildProjectStepFailureNamesItAndStillDropsTheIntermediateTag(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, projectDockerfileName) {
			return errors.New("exit status 1")
		}
		return nil
	})}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copying the project into the image failed")
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune astro-deploy/p-abc123:latest-deps")
}

// An interrupted build still drops the intermediate tag: the removal runs on
// a context the build's cancellation does not reach.
func TestBuildProjectDropsTheIntermediateTagWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var untagCtxErr error
	cmd := &ctxCmd{fakeCmd: fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, projectDockerfileName) {
			cancel()
			return context.Canceled
		}
		return nil
	})}, onUntag: func(c context.Context) { untagCtxErr = c.Err() }}

	_, err := New(cmd, func() time.Time { return fixedTime }).BuildLocal(ctx, projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune astro-deploy/p-abc123:latest-deps")
	assert.NoError(t, untagCtxErr, "the removal's context is not the canceled one")
}

// ctxCmd is fakeCmd that hands the context of the untag call to onUntag.
type ctxCmd struct {
	fakeCmd
	onUntag func(context.Context)
}

func (c *ctxCmd) Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	if len(args) > 1 && args[0] == "image" && args[1] == "rm" {
		c.onUntag(ctx)
	}
	return c.fakeCmd.Run(ctx, env, s, name, args...)
}

func TestBuildDependencyStepFailureStopsBeforeTheProject(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, "--tag astro-deploy/p-abc123:latest-deps") {
			return errors.New("exit status 1")
		}
		return nil
	})}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installing the project's dependencies")
	assert.Empty(t, callsContaining(cmd.calls, projectDockerfileName))
	assert.Empty(t, callsContaining(cmd.calls, "image rm"))
}

// A declared Dockerfile builds the project as its context already.
func TestBuildDockerfileModeIgnoresTheProjectContext(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1\n"), 0o600))
	req := projectRequest(t, project)
	req.Dockerfile, req.Context = filepath.Join(project, "Dockerfile"), project
	cmd := &fakeCmd{}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.NotContains(t, cmd.calls[0], "-deps")
}

func TestProjectIgnoreKeepsTheProjectsRulesAndPutsTheCLIsAfter(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, ".dockerignore"), []byte("secrets/\n!.env.example"), 0o600))

	got, err := ProjectBuilder{}.ProjectIgnore(project, []string{"dags"})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	assert.Equal(t, []string{"secrets/", "!.env.example"}, lines[:2], "the project's rules, unchanged and first")
	assert.Equal(t, "dags", lines[len(lines)-1], "the caller's excludes last")
	// 1.x's default .dockerignore, then v2's per-machine files.
	for _, rule := range []string{
		"astro", "logs", "airflow.db", "airflow.cfg",
		"**/.git", "**/airflow_settings.yaml", "**/.astro", "**/.venv", "**/.env", "**/.env.*", "**/.envrc",
		"**/__pycache__", "**/*.pyc", "plugins/fix_local_executor_pickle.py", "requirements.txt", "packages.txt",
	} {
		assert.Contains(t, lines, rule)
	}
	for _, line := range lines {
		assert.False(t, strings.HasPrefix(line, "/"), "%q: no leading slash, which buildah and Docker may read differently", line)
	}
}

func TestProjectIgnoreWithoutAProjectFile(t *testing.T) {
	got, err := ProjectBuilder{}.ProjectIgnore(t.TempDir(), nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, projectIgnoreHeader), got)
}

// Podman and buildah read .containerignore before .dockerignore, so on
// podman that file's rules are the project's; Docker never reads it.
func TestProjectIgnoreReadsContainerignoreOnPodman(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, ".dockerignore"), []byte("from-docker\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".containerignore"), []byte("from-container\n"), 0o600))

	got, err := ProjectBuilder{podman: true}.ProjectIgnore(project, nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "from-container\n"), got)
	assert.NotContains(t, got, "from-docker")

	got, err = ProjectBuilder{}.ProjectIgnore(project, nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "from-docker\n"), got)

	require.NoError(t, os.Remove(filepath.Join(project, ".containerignore")))
	got, err = ProjectBuilder{podman: true}.ProjectIgnore(project, nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "from-docker\n"), "without .containerignore podman reads .dockerignore: %s", got)
}

// The step that copies the project calls BeforeProjectCopy after the
// dependency build and before the copy, and stops on its error.
func TestBuildProjectLooksAgainBeforeTheCopy(t *testing.T) {
	var order []string
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, "--tag ") {
			order = append(order, "build "+map[bool]string{true: "deps", false: "project"}[strings.Contains(call, "-deps ")])
		}
		return nil
	})}
	req := projectRequest(t, t.TempDir())
	req.BeforeProjectCopy = func() error { order = append(order, "look"); return nil }
	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, []string{"build deps", "look", "build project"}, order)

	stop := errors.New("a credential appeared")
	req.BeforeProjectCopy = func() error { return stop }
	cmd = &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	_, err = testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, stop)
	assert.Empty(t, callsContaining(cmd.calls, projectDockerfileName), "nothing is copied")
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune", "the intermediate tag is still dropped")
}

// The check build's context has an ignore file of its own that leaves out
// nothing, and the CLI's leaves out the marker with a "**/" rule.
func TestCheckBuildContext(t *testing.T) {
	var seen map[string]string
	cmd := &fakeCmd{run: func(call string, s rt.Stdio) error {
		if _, dest, ok := strings.Cut(call, "--output type=local,dest="); ok {
			fields := strings.Fields(dest)
			ctxDir := fields[len(fields)-1]
			df := filepath.Join(filepath.Dir(fields[0]), "Dockerfile.check")
			seen = map[string]string{}
			for _, f := range []string{
				filepath.Join(ctxDir, ".dockerignore"), filepath.Join(ctxDir, ".containerignore"), df + ".dockerignore",
				filepath.Join(ctxDir, probeMarker), filepath.Join(ctxDir, probeSub, probeMarker),
			} {
				b, err := os.ReadFile(f)
				if err == nil {
					seen[filepath.Base(filepath.Dir(f))+"/"+filepath.Base(f)] = string(b)
				}
			}
			return checkBuild(fields[0], "honors")
		}
		return engine(dockerVersion, true, "", nil)(call, s)
	}}
	_, err := testBuilder(cmd).CanShipProject(context.Background(), Request{Bin: "docker"})
	require.NoError(t, err)
	assert.Equal(t, "unrelated\n", seen["context/.dockerignore"])
	assert.Equal(t, "unrelated\n", seen["context/.containerignore"])
	assert.Contains(t, seen, "context/"+probeMarker)
	assert.Contains(t, seen, probeSub+"/"+probeMarker)
	for k, v := range seen {
		if strings.HasSuffix(k, "Dockerfile.check.dockerignore") {
			assert.Equal(t, "**/"+probeMarker+"\n", v)
		}
	}
}

func TestShipProject(t *testing.T) {
	req := Request{}
	ShipProject(&req, "/p", true)
	assert.Equal(t, "/p", req.ProjectContext)
	assert.Empty(t, req.ProjectExcludes)

	ShipProject(&req, "/p", false)
	assert.Equal(t, []string{"dags"}, req.ProjectExcludes)

	declared := Request{Dockerfile: "/p/Dockerfile", Context: "/p"}
	ShipProject(&declared, "/p", false)
	assert.Empty(t, declared.ProjectContext, "a declared Dockerfile's context is the project already")
}

func TestDepsTag(t *testing.T) {
	for tag, want := range map[string]string{
		"astro-deploy/p-abc123":            "astro-deploy/p-abc123:latest-deps",
		"astro-package/p:src-1234567":      "astro-package/p:src-1234567-deps",
		"localhost:5000/team/p":            "localhost:5000/team/p:latest-deps",
		"localhost:5000/team/p:3.1-2-abcd": "localhost:5000/team/p:3.1-2-abcd-deps",
	} {
		assert.Equal(t, want, depsTag(tag), tag)
	}
}

// A builder that is not the docker driver (docker-container, remote,
// kubernetes) cannot see the local image the second build starts FROM, so
// the build is refused before anything is built.
func TestBuildProjectRefusesABuilderWithoutTheDockerDriver(t *testing.T) {
	cmd := &fakeCmd{run: engineWith(dockerVersion, true, "", "docker-container", "honors", nil)}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.ErrorIs(t, err, ErrNoProjectBuilder)
	assert.Contains(t, err.Error(), `"orbstack"`)
	assert.Empty(t, callsContaining(cmd.calls, "--tag"))
}

// A check build that copies the file its ignore file leaves out, or that
// fails, shows a builder that cannot be trusted with the ignore file: on
// Docker and on podman, a remote podman machine included, the build is
// refused before anything is built.
func TestBuildProjectRefusesABuilderThatFailsTheIgnoreFileCheck(t *testing.T) {
	for _, engineName := range []string{"docker", "podman"} {
		for _, check := range []string{"ignores", "fails", "empty", "rootonly", "nosub"} {
			t.Run(engineName+" "+check, func(t *testing.T) {
				version := dockerVersion
				if engineName == "podman" {
					version = "podman version 5.8.2"
				}
				cmd := &fakeCmd{run: engineWith(version, true, "--ignorefile", "docker", check, nil)}
				req := projectRequest(t, t.TempDir())
				req.Bin = engineName
				_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
				require.ErrorIs(t, err, ErrNoProjectBuilder)
				assert.Contains(t, err.Error(), "check build")
				assert.Empty(t, callsContaining(cmd.calls, "--tag"))
			})
		}
	}
}

// A podman check build is told the ignore file with --ignorefile.
func TestPodmanCheckBuildUsesIgnorefile(t *testing.T) {
	cmd := &fakeCmd{run: engine("podman version 5.8.2", false, "--ignorefile", nil)}
	_, err := testBuilder(cmd).CanShipProject(context.Background(), Request{Bin: "podman"})
	require.NoError(t, err)
	check := callsContaining(cmd.calls, "--output type=local")
	require.Len(t, check, 1)
	assert.Contains(t, check[0], "podman build --file ")
	assert.Contains(t, check[0], "--ignorefile ")
}

// The answer handed over as Request.Builder is used as it is: the engine is
// asked once.
func TestBuildUsesTheBuilderItIsHanded(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	pb, err := testBuilder(cmd).CanShipProject(context.Background(), Request{Bin: "docker"})
	require.NoError(t, err)
	asked := len(cmd.calls)

	req := projectRequest(t, t.TempDir())
	req.Builder = pb
	_, err = testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, callsContaining(cmd.calls[asked:], "--version"), "not asked again: %v", cmd.calls[asked:])
	assert.Empty(t, callsContaining(cmd.calls[asked:], "--output type=local"))
}

func TestCanShipProject(t *testing.T) {
	ok := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	_, err := testBuilder(ok).CanShipProject(context.Background(), Request{Bin: "docker"})
	require.NoError(t, err)
	assert.Empty(t, callsContaining(ok.calls, "--tag"), "a probe builds nothing")

	no := &fakeCmd{run: engine(dockerVersion, false, "", nil)}
	_, err = testBuilder(no).CanShipProject(context.Background(), Request{Bin: "docker"})
	require.ErrorIs(t, err, ErrNoProjectBuilder)
}
