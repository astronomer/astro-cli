package localdocker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	// nowhere else. They are typically decrypted connections and variables, and
	// a command line is readable by every local user through ps.
	environ := secretEnviron(req.Env)
	line := composeLine{
		conn:       conn,
		files:      composeFiles(path, projectPath),
		name:       name,
		projectDir: projectPath,
		// Compose resolves two kinds of valueless entry from this: the
		// PassthroughEnv and SecretEnv keys the file declares, and the bare
		// -e KEY flags below. A key it cannot resolve is dropped from the
		// container silently.
		extraEnv: environ,
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
	//
	// -e names each key with no value, so compose takes the value from its own
	// environment. The flag is still needed: Env is not limited to keys the
	// file declares, and one it does not declare would otherwise never reach
	// the container.
	args := []string{
		"run", "--rm", "--no-deps", "-T", "--pull", "never",
		"--entrypoint", req.Argv[0],
	}
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		args = append(args, "-e", key)
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
