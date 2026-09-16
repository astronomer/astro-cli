package localdocker

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // reproduces astro-cli v1's project-name hash; not a security boundary
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Everything in this file exists for one transition: a project that ran under
// `astro dev` before this runtime existed keeps its local Airflow database the
// first time it starts here. It has a death date — when no project has a v1
// volume left, the whole file goes.
//
// The problem it solves. Compose namespaces a named volume as
// <compose-project>_<volume>, both specs declare postgres_data, and the two
// paths derive the compose project name differently and can never coincide:
// v1 hashes the working directory with md5 behind the configured project name
// (airflow/container.go's ProjectNameUnique), this runtime uses
// composeProjectName. So the new stack asks for a volume that does not exist,
// compose makes an empty one, and Airflow comes up with no dag runs, no task
// instances, no XComs, no pools, none of the connections or Variables created
// in its UI, and no local admin user. Nothing was destroyed — the v1 volume is
// still on disk — but it is unreachable from the app, and it reads as "my local
// Airflow lost everything".
//
// Why copy rather than address the old volume by name. Compose can be told a
// volume's exact name, and pointing this runtime's postgres_data at the v1 name
// is fewer moving parts. It is also wrong here, for a reason that only shows up
// on a real machine: `astro dev stop` is `compose stop`, so a migrating project
// keeps its v1 containers, and an exited v1 postgres container still holds a
// reference to that volume. `down --volumes` then cannot remove it — compose
// reports "Resource is still in use" and leaves it — so every later reset would
// quietly fail to reset. Copying leaves the v1 volume and its containers exactly
// as they were, which also means the migration is reversible: `astro dev start`
// still finds its own database, and the copy is a backup rather than a move.
//
// The copy is a byte copy of the Postgres data directory, which is only sound
// because both runtimes pin the same image (postgresImage here, postgres.tag's
// default in v1) and the same superuser. Neither is assumed; both are checked.

// metadataVolumeKey is the volume this runtime's compose spec declares for the
// Postgres data directory, and the one v1's spec declared too. Compose
// namespaces it as <compose-project>_<key>.
//
// It must stay equal to the volume key in compose.yaml.tmpl: the copy writes to
// the name derived from it, and compose mounts the name derived from the
// template. TestMetadataVolumeKeyMatchesComposeTemplate pins them together.
const metadataVolumeKey = "postgres_data"

// legacyCleanupTimeout bounds the removal of a half-written volume, which runs
// on a context detached from the caller's.
const legacyCleanupTimeout = 30 * time.Second

// v1Config is the part of astro-cli v1's config this migration reads: the
// project name, which is half of the volume name, and the Postgres credentials,
// which decide whether the data is usable at all.
//
// Read directly rather than through astro-cli's config package because a pkg/
// sub-module may not import the parent module (docs/v2-architecture.md), and
// because this is three fields of a file that is on its way out.
type v1Config struct {
	Project struct {
		Name string `yaml:"name"`
	} `yaml:"project"`
	Postgres struct {
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"postgres"`
}

// readV1Config loads a project's config the way v1 resolved it: the project's
// own .astro/config.yaml, falling back per key to the home config, which is
// what cfg.GetString does. Reading only the project file would let a
// `astro config set -g postgres.user astro` slip past the credential guard
// below and produce a database whose superuser this runtime cannot log in as.
//
// A project without a config never ran under v1, so an absent file is "nothing
// to migrate" rather than an error.
func readV1Config(projectPath string) (v1Config, bool) {
	cfg, ok := readV1ConfigFile(filepath.Join(projectPath, ".astro", "config.yaml"))
	if !ok {
		return cfg, false
	}
	home, ok := readV1ConfigFile(v1HomeConfigPath())
	if !ok {
		return cfg, true
	}
	if cfg.Project.Name == "" {
		cfg.Project.Name = home.Project.Name
	}
	if cfg.Postgres.User == "" {
		cfg.Postgres.User = home.Postgres.User
	}
	if cfg.Postgres.Password == "" {
		cfg.Postgres.Password = home.Postgres.Password
	}
	return cfg, true
}

// v1HomeConfigPath mirrors config.initHome: $ASTRO_HOME/.astro/config.yaml when
// that is set, ~/.astro/config.yaml otherwise.
func v1HomeConfigPath() string {
	base := os.Getenv("ASTRO_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = home
	}
	return filepath.Join(base, ".astro", "config.yaml")
}

func readV1ConfigFile(path string) (v1Config, bool) {
	var cfg v1Config
	if path == "" {
		return cfg, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}

// validV1ImageName matches docker's grammar for a single image-name path
// component, copied from airflow/container.go so this reproduces v1's naming
// exactly rather than approximately.
var validV1ImageName = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// sanitizeV1ImageName is airflow/container.go's sanitizeImageName.
func sanitizeV1ImageName(s string) string {
	s = strings.ToLower(s)
	if validV1ImageName.MatchString(s) {
		return s
	}
	var b strings.Builder
	prevSep := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSep = false
		} else if !prevSep {
			b.WriteByte('-')
			prevSep = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	return out
}

// normalizeV1Name is airflow/container.go's normalizeName, which v1 applied on
// top of the above when it handed the name to compose.
var v1NameRunes = regexp.MustCompile("[a-z0-9_-]")

func normalizeV1Name(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(v1NameRunes.FindAllString(s, -1), "")
	return strings.TrimLeft(s, "_-")
}

// legacyComposeProject is the compose project name `astro dev` would have used
// for this project directory.
//
// v1 hashed os.Getwd() rather than a project path, which is the same thing
// here: its config loader looks for .astro/config.yaml in the working directory
// and does not search upward (config.initProject), so `astro dev start` only
// ever ran from a project root.
func legacyComposeProject(projectPath, projectName string) string {
	sum := md5.Sum([]byte(projectPath)) //nolint:gosec // G401: reproducing v1's hash, not protecting anything
	hash := hex.EncodeToString(sum[:])[:6]
	return normalizeV1Name(sanitizeV1ImageName(projectName + "_" + hash))
}

// legacyMetadataVolume is the volume `astro dev` would have used.
func legacyMetadataVolume(projectPath, projectName string) string {
	return legacyComposeProject(projectPath, projectName) + "_" + metadataVolumeKey
}

// legacyPathSpellings are the spellings of a project path that could have been
// hashed into a v1 volume name.
//
// composeProjectName resolves symlinks (rt.ProjectID -> CanonicalPath), so this
// runtime's name is stable across spellings while v1's is not: v1 hashed
// whatever os.Getwd() returned. A project reached through a symlink therefore
// has its v1 volume under one spelling and may be handed the other here, and a
// name that does not exist is a silent no-op — the worst outcome this file has.
// Both are tried; the second costs one lookup and only on a miss.
func legacyPathSpellings(projectPath string) []string {
	out := []string{projectPath}
	if resolved, err := filepath.EvalSymlinks(projectPath); err == nil && resolved != projectPath {
		out = append(out, resolved)
	}
	return out
}

// postgresMajor is the major version of a postgres image reference.
//
// The tag is the part after the LAST colon that follows the last slash, not the
// first colon in the string: a registry host may carry a port
// ("localhost:5000/postgres:12.6") and cutting at the first colon would yield
// nonsense.
func postgresMajor(image string) string {
	ref := image
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	_, tag, ok := strings.Cut(ref, ":")
	if !ok {
		return ""
	}
	major, _, _ := strings.Cut(tag, ".")
	return major
}

// volumeLookup is the three-way answer to "does this volume exist", because
// collapsing the third case into either of the first two is a data-loss bug in
// this file: "absent" is the branch that WRITES.
type volumeLookup int

const (
	volumeAbsent volumeLookup = iota
	volumePresent
	volumeUnknown
)

// lookupVolume asks one engine for a volume by exact name.
//
// `volume ls --filter` rather than `volume inspect`: inspect exits non-zero for
// both "no such volume" and "cannot reach the daemon", and Output discards
// stderr, so the two are indistinguishable. `ls` exits 0 whenever the daemon
// answered, which makes the distinction structural. The filter is a substring
// match on every engine, so the result is compared exactly rather than trusted.
func (e *Engine) lookupVolume(ctx context.Context, conn engineConn, name string) volumeLookup {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "volume", "ls", "--quiet", "--filter", "name="+name)
	if err != nil {
		return volumeUnknown
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return volumePresent
		}
	}
	return volumeAbsent
}

// findLegacyVolume looks for a v1 metadata volume across the engines and path
// spellings it could be under, and reports the engine that has it.
//
// Both engines, because v1 honored container.binary: a podman user's `astro
// dev` volumes live in podman, and e.preferred() may now resolve to docker.
// Asking only the preferred engine would make their database invisible with no
// message at all. Same order findProject uses — preferred first, the other only
// on a miss.
func (e *Engine) findLegacyVolume(ctx context.Context, conn engineConn, projectPath, projectName string) (engineConn, string, volumeLookup) {
	other := binPodman
	if conn.bin == binPodman {
		other = binDocker
	}
	unknown := false
	for i, c := range []engineConn{conn, e.connFor(other)} {
		for _, spelling := range legacyPathSpellings(projectPath) {
			vol := legacyMetadataVolume(spelling, projectName)
			switch e.lookupVolume(ctx, c, vol) {
			case volumePresent:
				return c, vol, volumePresent
			case volumeUnknown:
				// Only the preferred engine's silence is worth reporting: a
				// second engine that is simply not installed errors every time,
				// which is normal and not a reason to warn anybody.
				if i == 0 {
					unknown = true
				}
			case volumeAbsent:
			}
		}
	}
	if unknown {
		return conn, "", volumeUnknown
	}
	return conn, "", volumeAbsent
}

// volumeBusy reports whether a RUNNING container has this volume mounted.
//
// Exited containers are expected and fine to read past: `astro dev stop` leaves
// its postgres container behind, which is the ordinary state of a project being
// migrated. A running one is different — it means a live v1 stack, whose
// postgres is mid-write, and a byte copy of a data directory under an active
// server is a torn copy.
//
// The bool reports whether the engine answered at all, so the caller can tell
// "still running" from "could not ask" and say which.
func (e *Engine) volumeBusy(ctx context.Context, conn engineConn, name string) (busy, answered bool) {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "ps", "--quiet", "--filter", "volume="+name)
	if err != nil {
		return true, false
	}
	return len(bytes.TrimSpace(out)) > 0, true
}

// legacyAirflowMajor infers the Airflow generation of the v1 stack from the
// service names its containers still carry.
//
// v1's Airflow 2 spec had a webserver; its Airflow 3 spec has an api-server and
// a dag-processor, and this runtime's own airflowServices splits the same way.
// The image tag cannot answer: v1 built `<project>/airflow:latest`, which
// carries no version.
//
// Unknown when no v1 containers survive, which is possible — a prune removes
// them and leaves the volume.
func (e *Engine) legacyAirflowMajor(ctx context.Context, conn engineConn, project string) (string, bool) {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "ps", "--all",
		"--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}")
	if err != nil {
		return "", false
	}
	names := string(out)
	switch {
	case strings.Contains(names, "-api-server-"), strings.Contains(names, "-dag-processor-"):
		return "3", true
	case strings.Contains(names, "-webserver-"):
		return "2", true
	}
	return "", false
}

// adoptLegacyMetadataDB copies a project's `astro dev` metadata database into
// the volume this runtime is about to bring up, when it can establish that
// doing so is safe. It never fails a start: every path out is a decision not to
// migrate, reported to the frontend but not returned.
//
// Ordered so the common cases cost one `volume ls` and nothing else — a project
// that has already run here stops at the first check, and one that never ran
// under v1 stops at the third.
func (e *Engine) adoptLegacyMetadataDB(ctx context.Context, conn engineConn, projectPath, composeProject, major string, cb rt.Callbacks) {
	newVolume := composeProject + "_" + metadataVolumeKey

	// A volume under this runtime's own name means this project has started
	// here before, and whatever is in it is the current database. UNKNOWN stops
	// here too: "absent" is the branch that copies INTO this name, so reading a
	// daemon hiccup as "absent" would overwrite a live database with a v1
	// snapshot, and a failed copy would then remove the real one.
	if e.lookupVolume(ctx, conn, newVolume) != volumeAbsent {
		return
	}
	cfg, ok := readV1Config(projectPath)
	if !ok {
		return
	}
	legacyConn, legacy, found := e.findLegacyVolume(ctx, conn, projectPath, cfg.Project.Name)
	if found != volumePresent {
		return
	}
	legacyProject := strings.TrimSuffix(legacy, "_"+metadataVolumeKey)

	// From here the project has a v1 database and none here, so every remaining
	// exit is worth telling the user about: they are one step from losing sight
	// of real data, and silence is what made the original bug feel like
	// destruction.
	//
	// Each refusal names the same recovery, and it has to be this one. The start
	// continues after a refusal and compose creates an empty metadata volume, so
	// the first check above will skip adoption on every later start — a bare
	// "fix it and start again" could never work. Removing that volume is what
	// makes the retry possible, which is exactly what a reset does.
	const retry = "; fix that, run `astro local reset`, and start again to carry it over"

	if busy, answered := e.volumeBusy(ctx, legacyConn, legacy); busy {
		if answered {
			e.noteLegacyDB(cb, fmt.Sprintf(
				"not carrying over the local Airflow database in %s: its `astro dev` containers are still running, so copying it would tear the data directory — stop them with `astro dev stop`%s", legacy, retry))
		} else {
			e.noteLegacyDB(cb, fmt.Sprintf(
				"not carrying over the local Airflow database in %s: the container engine could not say whether its `astro dev` containers are still running%s", legacy, retry))
		}
		return
	}
	// v1 let the superuser and its password be configured, and the entrypoint
	// only applies POSTGRES_USER/POSTGRES_PASSWORD when it initializes an empty
	// data directory — so a copied one keeps whatever v1 created. This runtime's
	// connection string is fixed at postgres:postgres (postgresConn), so either
	// override produces a database that comes up and refuses every login.
	if u := cfg.Postgres.User; u != "" && u != "postgres" {
		e.noteLegacyDB(cb, fmt.Sprintf(
			"not carrying over the local Airflow database in %s: it was created for the Postgres user %q, and this runtime connects as postgres", legacy, u))
		return
	}
	if p := cfg.Postgres.Password; p != "" && p != "postgres" {
		e.noteLegacyDB(cb, fmt.Sprintf(
			"not carrying over the local Airflow database in %s: it was created with a custom postgres.password, and this runtime connects with the default", legacy))
		return
	}
	want := postgresMajor(postgresImage)
	got, readable := e.legacyPostgresMajor(ctx, legacyConn, legacy)
	switch {
	case !readable:
		e.noteLegacyDB(cb, fmt.Sprintf(
			"not carrying over the local Airflow database in %s: its Postgres version could not be read%s", legacy, retry))
		return
	case got != want:
		e.noteLegacyDB(cb, fmt.Sprintf(
			"not carrying over the local Airflow database in %s: it is Postgres %s data and this runtime runs Postgres %s", legacy, got, want))
		return
	}
	// Refusing a cross-generation carry rather than letting db-migration decide.
	// `airflow db migrate` against an Airflow 2 schema under an Airflow 3 image
	// is an irreversible upgrade of the user's real data that nobody asked for
	// here, and when it instead aborts on a compatibility check every service's
	// service_completed_successfully dependency fails and the project cannot
	// start at all — after being told the database was carried over.
	if lm, known := e.legacyAirflowMajor(ctx, legacyConn, legacyProject); known && lm != major {
		e.noteLegacyDB(cb, fmt.Sprintf(
			"not carrying over the local Airflow database in %s: it belongs to an Airflow %s project and this one runs Airflow %s, which would rewrite its schema", legacy, lm, major))
		return
	}
	// A volume the engine creates carries no compose labels, and a teardown
	// enumerates a project's volumes by label: downProject runs `down` with no
	// --file (see composeLine.argv), so compose rebuilds the project from labels
	// alone and an unlabeled volume is invisible to `down --volumes`. Without
	// these, `astro local reset` and `stop --clean` would report a wipe and
	// leave the migrated database on disk — the very failure copying was chosen
	// to avoid. They also suppress compose's "volume X already exists but was
	// not created by Docker Compose" notice on every start.
	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "volume", "create",
		"--label", "com.docker.compose.project="+composeProject,
		"--label", "com.docker.compose.volume="+metadataVolumeKey,
		newVolume,
	); err != nil {
		e.noteLegacyDB(cb, fmt.Sprintf("could not carry over the local Airflow database in %s: %s", legacy, err))
		return
	}
	// cp -a preserves the ownership and modes inside the data directory; the
	// mount point itself is created by the engine as root-owned 0755, which
	// Postgres refuses to start on, so it is fixed explicitly. postmaster.pid
	// and postmaster.opts are dropped: a v1 stack that was killed rather than
	// stopped leaves a stale lock file, and Postgres can refuse to start on one
	// whose recorded PID happens to exist in the new container.
	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "run", "--rm",
		"-v", legacy+":/src:ro", "-v", newVolume+":/dst", postgresImage,
		"sh", "-c", "cp -a /src/. /dst/ && rm -f /dst/postmaster.pid /dst/postmaster.opts && chown postgres:postgres /dst && chmod 700 /dst",
	); err != nil {
		e.removeHalfWrittenVolume(ctx, conn, newVolume)
		e.noteLegacyDB(cb, fmt.Sprintf("could not carry over the local Airflow database in %s: %s", legacy, err))
		return
	}
	e.noteLegacyDB(cb, fmt.Sprintf(
		"carried over the local Airflow database from `astro dev` (%s); the original is left on disk untouched", legacy))
}

// removeHalfWrittenVolume drops a destination volume whose copy failed.
//
// A half-written data directory is worse than none: Postgres would fail to
// start on it, and the first check in adoptLegacyMetadataDB reads any existing
// volume as "already migrated", so it would never be repaired — the project
// could not start again without a manual volume rm.
//
// Detached from the caller's context for the same reason rollback is: the
// desktop drives docker under a deadline, so the copy frequently fails BECAUSE
// ctx is already done, and os/exec refuses to spawn on a canceled context.
// Reusing it would make this a silent no-op in exactly the case it exists for.
func (e *Engine) removeHalfWrittenVolume(ctx context.Context, conn engineConn, name string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), legacyCleanupTimeout)
	defer cancel()
	//nolint:errcheck // best effort: the copy already failed, and the start
	// carries on either way with whatever the removal left behind.
	e.cmd.Output(ctx, conn.env, conn.bin, "volume", "rm", "--force", name)
}

// legacyPostgresMajor reads PG_VERSION out of the v1 volume.
//
// This is the authoritative compatibility check, and it is deliberately
// evidence rather than inference: v1 took its image from postgres.repository
// and postgres.tag, which could be set in the project's config or the global
// one, so reading the version the data was actually written by covers every way
// it could have been overridden.
func (e *Engine) legacyPostgresMajor(ctx context.Context, conn engineConn, volume string) (string, bool) {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "run", "--rm",
		"-v", volume+":/src:ro", postgresImage, "cat", "/src/PG_VERSION")
	if err != nil {
		return "", false
	}
	major, _, _ := strings.Cut(strings.TrimSpace(string(out)), ".")
	return major, major != ""
}

// noteLegacyDB reports a migration decision on the runtime's own channel. The
// runtime never prints (docs/v2-architecture.md); each frontend renders this.
func (e *Engine) noteLegacyDB(cb rt.Callbacks, text string) {
	if cb.OnLine != nil {
		cb.OnLine(rt.LogLine{Component: "system", Time: e.now(), Text: text})
	}
}
