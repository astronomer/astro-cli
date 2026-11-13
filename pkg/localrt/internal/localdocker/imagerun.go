package localdocker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// RunInImage runs one command in the project's image, with no Airflow running.
//
// Everything it needs — which image, which project directories are mounted
// where, what environment Airflow runs with — is already in the compose file
// this engine wrote at the last start, so compose supplies all of it and none
// of it is re-derived here. Re-deriving would mean naming the built image by a
// rule only this package knows, and that rule has a case a second copy would
// get wrong: with nothing extra to install the builder builds nothing and hands
// back the base runtime image, so the per-project tag never exists. The
// project's override is merged over it as it is now, not as it was at that
// start, so an override the up would reject fails this too.
//
// That file is also what makes this work while the project is down. A plain
// stop removes the state record and leaves the compose file and the image in
// place, which is exactly the state this exists for; a --clean stop removes
// both, and then there is nothing to run.
//
// The invocation is built through composeLine like every other compose command
// here, which is what gets it the project name and project directory. Those are
// not decoration: without the name, compose derives one from the compose file's
// own directory, and the containers and networks it creates then sit under a
// name no teardown sweeps.
func (e *Engine) RunInImage(ctx context.Context, projectPath string, req rt.ImageRun) error {
	if len(req.Argv) == 0 {
		return errors.New("no command given")
	}
	// One project at a time, and the same lock Start holds while it writes the
	// compose file. writeComposeFile truncates in place rather than writing a
	// temp file and renaming, so reading it unlocked can catch a concurrent
	// start mid-write — which surfaces as a YAML parse error blamed on this
	// command rather than on the race.
	unlock, err := localstate.Lock(projectPath)
	if err != nil {
		return err
	}
	defer unlock()

	// A record for another mode means the compose file, if there is one, is a
	// leftover from a previous docker start. Running the command in that stale
	// image would answer confidently from the wrong dependencies.
	if rec, err := localstate.Load(projectPath); err == nil && rec.Mode != rt.ModeDocker {
		return fmt.Errorf("this project's local Airflow is running in %s mode, not docker", rec.Mode)
	}

	path, err := e.composeFileFor(projectPath)
	if err != nil {
		return err
	}
	// Only a missing file is "no image yet". A permission or I/O fault is not
	// something starting the project fixes, and reporting it as one sends the
	// user somewhere that cannot help.
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: start it once to build one", rt.ErrImageNotBuilt)
		}
		return fmt.Errorf("reading the project's compose file: %w", err)
	}

	// The same two pre-flights Start does, and for a sharper reason: this
	// command's whole purpose is a project that is NOT running, which is the
	// state where the engine is most likely to be down too. Without them the
	// user gets "Cannot connect to the Docker daemon" or an opaque exit 125
	// instead of the auto-start, or the actionable missing-plugin error.
	if err := e.ensureEngine(rt.Callbacks{}, projectPath); err != nil {
		return err
	}
	conn, err := e.preferred(projectPath)
	if err != nil {
		return err
	}
	if err := e.composeAvail(ctx, conn); err != nil {
		return err
	}

	name, err := composeProjectName(projectPath)
	if err != nil {
		return err
	}
	// The caller's values travel in the compose process's environment and
	// nowhere else, under their secretRef aliases. They are typically decrypted
	// connections and variables: a command line is readable by every local user
	// through ps, and under its own name a value like DOCKER_HOST would retarget
	// this compose call instead of reaching the container.
	values, err := imageRunValues(path, req.Env)
	if err != nil {
		return err
	}
	files := composeFiles(path, projectPath)
	// The caller's keys reach the container through a run-only override that
	// declares each as a reference to its alias. Env is not limited to keys the
	// generated file declares, and one it does not declare would otherwise
	// never reach the container. The override holds names, never values.
	if override, err := writeImageRunOverride(filepath.Dir(path), req.Env); err != nil {
		return err
	} else if override != "" {
		defer os.Remove(override) //nolint:errcheck // best-effort cleanup of a file holding no values
		files = append(files, override)
	}
	line := composeLine{
		conn:       conn,
		files:      files,
		name:       name,
		projectDir: projectPath,
		extraEnv:   secretEnviron(values),
	}

	// --pull never: the image was built locally and never pushed, so a pull can
	// only fail — slowly, over the network, from an operation documented as
	// offline. The image can be gone while the compose file remains (an engine
	// prune, a rebuilt VM), and this is what turns that into a clear failure.
	//
	// -T keeps stdout and stderr apart: compose allocates a pseudo-terminal
	// when attached to one, and a caller parsing stdout would be parsing the
	// other stream with it.
	//
	// --no-deps runs nothing but this container: the command has no use for the
	// metadata database, and starting one would be a side effect. --rm leaves
	// nothing behind.
	//
	// --entrypoint replaces the image's, which waits for the metadata database
	// and prints to stdout while it waits. Overriding it also discards the
	// service's own command, so the arguments below are the whole command line
	// and a program with no arguments runs as itself.
	args := []string{
		"run", "--rm", "--no-deps", "-T", "--pull", "never",
		"--entrypoint", req.Argv[0],
	}
	args = append(args, execService)
	args = append(args, req.Argv[1:]...)

	return e.cmd.Run(ctx, line.env(), req.Stdio, conn.bin, line.argv(args...)...)
}

// composeFileFor is the generated compose file's path for a project. Unlike the
// airflow handle's version it takes a path, because nothing is running here to
// have a state record.
func (e *Engine) composeFileFor(projectPath string) (string, error) {
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, composeFileName), nil
}

// secretRefPattern finds the aliases a generated compose file references.
var secretRefPattern = regexp.MustCompile(secretRefPrefix + `[0-9a-f]+`)

// imageRunValues are the values a run hands compose, keyed by the variable
// they belong to: every key in env, plus every SecretEnv key the compose file
// references that env does not hold, taken from the CLI's own environment.
//
// The second half keeps what the file's declarations resolved to before they
// were references: a valueless `KEY:` took the run's environment, which is
// the CLI's own with env over it. A key neither holds interpolates as empty,
// where the valueless form left it unset.
func imageRunValues(composePath string, env map[string]string) (map[string]string, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("reading the project's compose file: %w", err)
	}
	values := make(map[string]string, len(env))
	for _, ref := range secretRefPattern.FindAllString(string(data), -1) {
		key, ok := secretKey(ref)
		if !ok {
			continue
		}
		if v, ok := os.LookupEnv(key); ok {
			values[key] = v
		}
	}
	for k, v := range env {
		values[k] = v
	}
	return values, nil
}

// writeImageRunOverride writes, in dir, a compose override declaring each key
// in env on the service a run uses, as a reference to its alias, and returns
// its path, or "" when env names nothing to declare. It holds key names only:
// the values are interpolated from the compose process's environment.
//
// One fixed name rather than a temp file: RunInImage holds the project lock,
// so no two runs share it at once, and a file a killed run left behind is
// overwritten by the next one instead of accumulating.
func writeImageRunOverride(dir string, env map[string]string) (string, error) {
	keys := make([]string, 0, len(env))
	for k := range env {
		if secretKeyOK(k) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return "", nil
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("services:\n  " + execService + ":\n    environment:\n")
	for _, k := range keys {
		b.WriteString("      " + quoteYAML(k) + ": " + secretInterpolation(secretRef(k)) + "\n")
	}
	path := filepath.Join(dir, imageRunOverrideName)
	if err := os.WriteFile(path, []byte(b.String()), imageRunOverridePerm); err != nil {
		return "", fmt.Errorf("writing the run's compose override: %w", err)
	}
	return path, nil
}

// imageRunOverrideName is the run-only compose override in the state dir, and
// imageRunOverridePerm its mode: the CLI user's alone, like the state dir.
const (
	imageRunOverrideName = "compose.run.yaml"
	imageRunOverridePerm = 0o600
)
