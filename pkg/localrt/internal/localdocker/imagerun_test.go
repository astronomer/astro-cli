package localdocker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// withComposeFile puts a generated compose file where the engine looks for one,
// which is what makes a project runnable while it is down.
func withComposeFile(t *testing.T, projectPath string) string {
	t.Helper()
	dir, err := rt.StateDir(projectPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, composeFileName)
	require.NoError(t, os.WriteFile(path, []byte("services: {}\n"), 0o600))
	return path
}

// The command is built so its output is actually usable, and so it runs nothing
// but itself.
func TestRunInImageShapesACommandWhoseOutputCanBeParsed(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composePath := withComposeFile(t, project)

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python", "-", "/usr/local/airflow"},
	}))

	require.Len(t, cmd.calls, 1)
	call := cmd.calls[0]

	// -T is the whole reason a caller can parse stdout: compose allocates a
	// pseudo-terminal when attached to one, and that merges stdout with stderr.
	assert.Contains(t, call, " -T ",
		"without -T compose merges stdout and stderr and the output cannot be parsed")
	// The image's entrypoint waits for the metadata database and prints while
	// it waits, so it must be bypassed or the command hangs and is corrupted.
	assert.Contains(t, call, "--entrypoint python")
	// Nothing else comes up: no database, and nothing left behind.
	assert.Contains(t, call, "--no-deps")
	assert.Contains(t, call, "--rm")
	// The compose file is the source of the image, the mounts and the env.
	assert.Contains(t, call, "--file "+composePath)
	// The image was built locally and never pushed, so a pull can only fail —
	// slowly, over the network, from an operation documented as offline.
	assert.Contains(t, call, "--pull never")

	// The project name and directory are what tie this one-off to the project
	// every other command addresses. Without the name compose derives one from
	// the compose file's own directory, and whatever it creates — the scheduler
	// service declares a network — sits under a name no teardown sweeps.
	// Without the directory compose looks for the project's .env somewhere it
	// is not, and mislabels working_dir, which findProject discovers by.
	wantName, err := composeProjectName(project)
	require.NoError(t, err)
	assert.Contains(t, call, "--project-name "+wantName)
	assert.Contains(t, call, "--project-directory "+project)

	// The program's own arguments follow the service, not the flags.
	service := strings.Index(call, " "+execService+" ")
	require.Positive(t, service, "the service should be named in %q", call)
	assert.Equal(t, "- /usr/local/airflow",
		strings.TrimSpace(call[service+len(execService)+2:]),
		"argv after the program name belongs after the service")
}

// A project that has never been started, or was stopped with --clean, has no
// image. That is its own answer, and it is reported without running anything.
func TestRunInImageRefusesWithoutAnImage(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)

	err := e.RunInImage(context.Background(), t.TempDir(), rt.ImageRun{Argv: []string{"python"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, rt.ErrImageNotBuilt)
	assert.Empty(t, cmd.calls, "nothing should run when there is no image")
}

func TestRunInImageRequiresACommand(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	require.Error(t, e.RunInImage(context.Background(), project, rt.ImageRun{}))
	assert.Empty(t, cmd.calls)
}

// Extra environment reaches compose under its aliases, declared to the
// container by a run-only override the run cleans up, and a key that cannot
// survive KEY=VALUE is dropped rather than declared-but-unresolved.
func TestRunInImagePassesEnvironmentDeterministically(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
		Env: map[string]string{
			"ZED":          "last",
			"AIRFLOW_HOME": "/tmp/scratch",
			"BAD=KEY":      "dropped",
			"":             "dropped too",
		},
	}))

	require.Len(t, cmd.calls, 1)
	call := cmd.calls[0]
	assert.Contains(t, call, "/compose.run-", "the caller's keys are declared by a run-only override")
	assert.NotContains(t, call, " -e ", "no key is named on the command line")
	assert.NotContains(t, call, "BAD", "a key holding = would never resolve")

	env, ok := cmd.envFor("run --rm")
	require.True(t, ok, "the run command's environment was not recorded")
	assert.Equal(t, []string{secretRef("AIRFLOW_HOME") + "=/tmp/scratch", secretRef("ZED") + "=last"}, env,
		"each value travels under its alias, in order, and nothing else does")

	dir, err := rt.StateDir(project)
	require.NoError(t, err)
	leftovers, err := filepath.Glob(filepath.Join(dir, "compose.run-*.yaml"))
	require.NoError(t, err)
	assert.Empty(t, leftovers, "the run's override is removed when the run ends")
}

// The override declares each caller key on the run's service as a reference
// to its alias, sorted, and holds no value.
func TestWriteImageRunOverrideDeclaresReferencesOnly(t *testing.T) {
	dir := t.TempDir()
	path, err := writeImageRunOverride(dir, map[string]string{"ZED": "s3cret", "DOCKER_HOST": "tcp://x:2375", "BAD=KEY": "v"})
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "services:\n  "+execService+":\n    environment:\n"+
		"      'DOCKER_HOST': "+secretInterpolation(secretRef("DOCKER_HOST"))+"\n"+
		"      'ZED': "+secretInterpolation(secretRef("ZED"))+"\n", string(data))

	none, err := writeImageRunOverride(dir, nil)
	require.NoError(t, err)
	assert.Empty(t, none, "nothing to declare writes nothing")
}

// A secret the compose file declares from the last start, and the caller does
// not pass, takes the CLI's own value for it, as the valueless entry it
// replaced did; the caller's value wins one it does pass.
func TestRunInImageResolvesTheFilesReferencesAsBefore(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	path := withComposeFile(t, project)
	file := "x-common-env: &common-env\n" +
		"  FROM_SHELL: " + secretInterpolation(secretRef("FROM_SHELL")) + "\n" +
		"  FROM_CALLER: " + secretInterpolation(secretRef("FROM_CALLER")) + "\n" +
		"  NOWHERE: " + secretInterpolation(secretRef("NOWHERE")) + "\n" +
		"services: {}\n"
	require.NoError(t, os.WriteFile(path, []byte(file), 0o600))
	t.Setenv("FROM_SHELL", "shell-value")
	t.Setenv("FROM_CALLER", "loses")

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
		Env:  map[string]string{"FROM_CALLER": "caller-value"},
	}))

	env, ok := cmd.envFor("run --rm")
	require.True(t, ok)
	assert.Contains(t, env, secretRef("FROM_SHELL")+"=shell-value")
	assert.Contains(t, env, secretRef("FROM_CALLER")+"=caller-value")
	for _, kv := range env {
		assert.False(t, strings.HasPrefix(kv, secretRef("NOWHERE")+"="), "a key nothing holds gets no value")
	}
}

// Values never appear on the command line, where any local user can read them
// through ps. They reach the container only through the compose process's
// environment, under the aliases the run's override references.
func TestRunInImageKeepsValuesOffTheCommandLine(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	secrets := map[string]string{
		"AIRFLOW_CONN_WAREHOUSE": "postgres://user:hunter2-conn@db:5432/x",
		"AIRFLOW_VAR_API_TOKEN":  "tok-hunter2-var",
		"SNOWFLAKE_PASSWORD":     "pw-hunter2-env",
	}
	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python", "-c", "pass"},
		Env:  secrets,
	}))

	require.Len(t, cmd.calls, 1)
	call := cmd.calls[0]
	env, ok := cmd.envFor("run --rm")
	require.True(t, ok, "the run command's environment was not recorded")
	for k, v := range secrets {
		// The value itself is never printed: a failure here names the key and
		// says how long the leaked value was, which is enough to find it.
		assert.Truef(t, !strings.Contains(call, v),
			"the value of %s (%d bytes) is on the command line", k, len(v))
		assert.Truef(t, slices.Contains(env, secretRef(k)+"="+v),
			"the value of %s should be in the compose process environment, under its alias", k)
	}
}

// The caller's streams are the command's streams: the program arrives on stdin
// and its output comes back where the caller asked for it.
func TestRunInImageWiresTheCallersStreams(t *testing.T) {
	var gotStdin []byte
	cmd := &fakeCmd{run: func(_ string, s rt.Stdio) error {
		var err error
		gotStdin, err = io.ReadAll(s.In)
		require.NoError(t, err)
		_, _ = s.Out.Write([]byte(`{"dags":[]}`))
		_, _ = s.Err.Write([]byte("a log line"))
		return nil
	}}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	var out, errBuf bytes.Buffer
	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv:  []string{"python", "-"},
		Stdio: rt.Stdio{In: strings.NewReader("print('hi')"), Out: &out, Err: &errBuf},
	}))

	assert.Equal(t, "print('hi')", string(gotStdin), "the program is fed on stdin")
	assert.Equal(t, `{"dags":[]}`, out.String())
	assert.Equal(t, "a log line", errBuf.String(), "log noise stays out of the parsed stream")
}

// A failing command's error reaches the caller rather than being reported as a
// missing image, so the two states stay distinguishable.
func TestRunInImageReportsACommandFailure(t *testing.T) {
	boom := errors.New("exit status 1")
	cmd := &fakeCmd{run: func(string, rt.Stdio) error { return boom }}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	err := e.RunInImage(context.Background(), project, rt.ImageRun{Argv: []string{"python"}})
	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, rt.ErrImageNotBuilt)
}

// The container-side project path matches the runtime image's AIRFLOW_HOME.
//
// This is a plain literal on purpose. It was written as a comparison against
// projectMounts' output, which could not fail: this package's own mount root is
// DEFINED as rt.ProjectDirInImage, so the assertion compared the constant with
// itself and passed with the constant set to "/opt/mutant".
//
// What the value actually has to agree with is outside this repo — the Astro
// Runtime image keeps AIRFLOW_HOME at this path, and the mounts and every
// caller's arguments are placed relative to it. Nothing in Go can check that,
// so the literal is the checkpoint: changing it has to be a deliberate edit
// here, with the image checked by hand.
func TestTheContainerProjectPathMatchesTheRuntimeImage(t *testing.T) {
	assert.Equal(t, "/usr/local/airflow", rt.ProjectDirInImage)

	// And the mounts are placed relative to it rather than to a second copy.
	project := t.TempDir()
	for _, d := range mountDirs {
		require.NoError(t, os.MkdirAll(filepath.Join(project, d), 0o755))
	}
	for _, m := range projectMounts(project) {
		assert.Equal(t, rt.ProjectDirInImage+"/"+filepath.Base(m.Host), m.Container)
	}
}

// The engine is brought up and the compose plugin checked BEFORE any compose
// command, the same two pre-flights Start does.
//
// Sharper here than in Start: this command's whole purpose is a project that is
// not running, which is the state where the engine is most likely to be down
// too — a fresh login, Docker Desktop not launched. Without these the user gets
// "Cannot connect to the Docker daemon" or an opaque exit 125 instead of the
// auto-start, or the actionable missing-plugin error.
func TestRunInImageBringsTheEngineUpBeforeRunning(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	var order []string
	e.ensureEngine = func(rt.Callbacks, string) error { order = append(order, "ensureEngine"); return nil }
	e.composeAvail = func(context.Context, engineConn) error {
		order = append(order, "composeAvail")
		return nil
	}

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
	}))
	assert.Equal(t, []string{"ensureEngine", "composeAvail"}, order)
	assert.Len(t, cmd.calls, 1, "the command should still run once both pass")
}

func TestRunInImageStopsWhenTheEngineWillNotStart(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	down := errors.New("the docker daemon is not running")
	e.ensureEngine = func(rt.Callbacks, string) error { return down }

	require.ErrorIs(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
	}), down)
	assert.Empty(t, cmd.calls, "no compose command should run against a dead engine")
}

func TestRunInImageStopsWithoutTheComposePlugin(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)

	missing := errors.New("compose v2 is not installed")
	e.composeAvail = func(context.Context, engineConn) error { return missing }

	require.ErrorIs(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
	}), missing)
	assert.Empty(t, cmd.calls)
}

// A project whose Airflow is recorded in another mode is refused rather than
// run in a leftover image.
//
// The compose file survives a plain stop, so a project started once with
// --docker and since converted to standalone still has one. Running the
// command in that image would answer confidently from the wrong interpreter
// and the wrong pinned dependencies, with nothing reporting the mismatch.
func TestRunInImageRefusesARecordFromAnotherMode(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        rt.ModeStandalone,
		PID:         os.Getpid(),
	}))

	err := e.RunInImage(context.Background(), project, rt.ImageRun{Argv: []string{"python"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not docker")
	assert.NotErrorIs(t, err, rt.ErrImageNotBuilt,
		"a standalone project is not a docker project missing an image")
	assert.Empty(t, cmd.calls)
}

// A docker record is the expected case and does not get in the way.
func TestRunInImageAcceptsADockerRecord(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	withComposeFile(t, project)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        rt.ModeDocker,
		PID:         os.Getpid(),
	}))

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
	}))
	assert.Len(t, cmd.calls, 1)
}

// The caller's environment reaches the compose PROCESS under aliases only, so a
// caller-supplied DOCKER_HOST cannot retarget the invocation, whether or not
// the connection names one: the only DOCKER_HOST compose sees is the
// connection's. The container still gets the caller's value, through the
// override's reference, as the running Airflow does after a start.
func TestRunInImageGivesTheCallersEnvToComposeItself(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	e.preferred = func(string) (engineConn, error) {
		return engineConn{bin: "docker", env: []string{"DOCKER_HOST=unix:///real.sock"}}, nil
	}
	project := t.TempDir()
	withComposeFile(t, project)

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{
		Argv: []string{"python"},
		Env: map[string]string{
			"AIRFLOW_VAR_TOKEN": "s3cret",
			"DOCKER_HOST":       "unix:///attacker.sock",
		},
	}))

	env, ok := cmd.envFor("run --rm")
	require.True(t, ok, "the run command's environment was not recorded")
	assert.Contains(t, env, secretRef("AIRFLOW_VAR_TOKEN")+"=s3cret")
	assert.Contains(t, env, secretRef("DOCKER_HOST")+"=unix:///attacker.sock",
		"the caller's DOCKER_HOST travels under its alias, for the container")
	var hosts []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "DOCKER_HOST=") {
			hosts = append(hosts, kv)
		}
	}
	assert.Equal(t, []string{"DOCKER_HOST=unix:///real.sock"}, hosts,
		"the only DOCKER_HOST compose itself sees is the connection's")
	require.Len(t, cmd.calls, 1)
	assert.NotContains(t, cmd.calls[0], "attacker.sock", "no value is on the command line")
}

// A compose file that stat cannot resolve for some other reason is not reported
// as a missing image: starting the project would not fix it, and sending the
// user there hides the real cause.
//
// A symlink loop, not a permission fault. The first attempt used chmod 000 on
// the state directory and passed for the wrong reason — localstate.Lock opens a
// lock file in that same directory and fails before the stat is ever reached,
// so the assertion held whatever the stat branch did. A loop leaves the
// directory usable and breaks only this one path.
func TestRunInImageDistinguishesAnUnresolvableComposeFileFromAMissingOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on Windows")
	}
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()

	dir, err := rt.StateDir(project)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, composeFileName)
	require.NoError(t, os.Symlink(path, path), "a symlink to itself")

	// Confirm the construction actually produces the error class under test,
	// rather than assuming it does.
	_, statErr := os.Stat(path)
	require.Error(t, statErr)
	require.NotErrorIs(t, statErr, fs.ErrNotExist, "the loop should not read as absent")

	err = e.RunInImage(context.Background(), project, rt.ImageRun{Argv: []string{"python"}})
	require.Error(t, err)
	assert.NotErrorIs(t, err, rt.ErrImageNotBuilt,
		"an unresolvable path is not a project that has never been started")
	assert.Empty(t, cmd.calls)
}
