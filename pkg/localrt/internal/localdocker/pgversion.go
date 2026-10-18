package localdocker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Which Postgres a project's metadata database runs.
//
// A new database gets postgresImage. An existing one keeps the major version
// that created it: Postgres refuses to start on a data directory an older major
// initialized ("database files are incompatible with server"), and moving one
// across majors takes a dump and a restore. Every docker-mode volume made
// before the default moved to 15 was initialized by legacyPostgresImage, and a
// plain stop keeps that volume by design, so bumping the image for everyone
// would have left each of those projects unable to start.
//
// The version is read from the data directory itself. Postgres writes the
// major into PG_VERSION when it creates the directory, so the answer is right
// for every volume that exists today and for one that outlived the state
// directory, which lives in a cache anything may clear. What was read is
// remembered in the state directory, keyed by the volume's creation time, so
// later starts of the same volume skip the read.
//
// An existing volume whose version cannot be read runs legacyPostgresImage,
// not the new default: every volume this runtime made before 15 became the
// default holds a 12 database, and 15 on one of those never comes up — compose
// waits on the migration for as long as Postgres keeps refusing.
//
// Moving a project onto the new default is a reset: it deletes the volume, and
// the next start creates the database on postgresImage. The database is local
// development data, which is why nothing here offers to carry it across.

const (
	// postgresImage is what a new metadata database is created with, and the
	// major Astro CLI 1.x moved its own default to.
	postgresImage = postgresRepository + ":" + postgresMajor
	postgresMajor = "15"

	// legacyPostgresImage is what every docker-mode database was created with
	// before that: v1's default, which this runtime adopted unchanged. A
	// project still on 12 keeps this exact tag rather than the newest 12, so
	// it runs the image it already has and an upgrade costs it no pull.
	legacyPostgresImage = postgresRepository + ":12.6"
	legacyPostgresMajor = "12"

	postgresRepository = "docker.io/postgres"

	// pgDataDir is where the compose template mounts the metadata volume, and
	// PG_VERSION the file in it holding the major version that created it.
	pgDataDir     = "/var/lib/postgresql/data"
	pgVersionFile = "PG_VERSION"

	// pgProbeSuffix names the container the version is read through, after the
	// compose project it belongs to. A fixed name rather than a generated one
	// so a probe an interrupted start left behind is found and removed by the
	// next start or reset: it holds the volume, and docker will not delete a
	// volume a container still refers to.
	pgProbeSuffix = "-pgversion"

	// pgRecordFile is where the state directory remembers what was read.
	pgRecordFile = "postgres.json"

	// postgresService is the compose template's database service.
	postgresService = "postgres"
)

// pgProbeCleanupTimeout bounds removing the probe container once the version
// is read. It runs on a context detached from the start's, since a start that
// was canceled mid-probe still has to take its probe back out.
const pgProbeCleanupTimeout = 30 * time.Second

// metadataVolume is the name compose gives a project's metadata volume.
func metadataVolume(composeProject string) string {
	return composeProject + "_" + metadataVolumeKey
}

// pgProbeName is the probe container's name for a compose project.
func pgProbeName(composeProject string) string {
	return composeProject + pgProbeSuffix
}

// pgRecord is what the state directory remembers about one metadata volume.
//
// CreatedAt is the key, beside the engine and the volume's name: a reset
// deletes the volume and the next start makes a new one under the same name,
// and a record that matched by name alone would hand the new database the old
// one's version. Noted is whether the start has said why this volume is not on
// the default, which it says once per volume.
type pgRecord struct {
	Engine    string `json:"engine"`
	Volume    string `json:"volume"`
	CreatedAt string `json:"createdAt"`
	Version   string `json:"version"`
	Noted     bool   `json:"noted,omitempty"`
}

func (r pgRecord) matches(engine, volume, createdAt string) bool {
	return createdAt != "" && r.Engine == engine && r.Volume == volume && r.CreatedAt == createdAt
}

// loadPGRecord reads the record, or the zero record when there is none or it
// cannot be read: it only ever saves a probe, so a bad one costs one.
func loadPGRecord(stateDir string) pgRecord {
	var r pgRecord
	b, err := fsatomic.ReadFile(filepath.Join(stateDir, pgRecordFile))
	if err != nil || json.Unmarshal(b, &r) != nil {
		return pgRecord{}
	}
	if _, err := parsePGVersion(r.Version); err != nil {
		return pgRecord{}
	}
	return r
}

// savePGRecord writes the record. Best effort: losing it costs the next start
// a probe and, at worst, the note again.
func savePGRecord(stateDir string, r pgRecord) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	//nolint:errcheck // best effort by design; see the doc comment
	_ = fsatomic.WriteFile(filepath.Join(stateDir, pgRecordFile), b, proxy.FilePermRW)
}

// pgChoice is where the Postgres choice for one start is made.
type pgChoice struct {
	conn           engineConn
	stateDir       string
	projectPath    string
	composeProject string
	// probeImage is any image the engine has or can get; see readPGVersion.
	probeImage string
	// own is whether the project's metadata volume exists, asked before
	// anything in this start could have created it.
	own volumeLookup
}

// metadataPostgresImage picks the image the project's metadata database runs:
// postgresImage for a new database, and for an existing one the major version
// it was created with.
func (e *Engine) metadataPostgresImage(ctx context.Context, c pgChoice, cb rt.Callbacks) string {
	if overrideSetsPostgresImage(c.projectPath) {
		// The override's image is what runs: it merges over this file. Reading
		// the version would cost a probe to choose an image nobody uses, and a
		// note about it would promise what the override decides.
		return postgresImage
	}
	switch c.own {
	case volumeAbsent:
		return postgresImage
	case volumeUnknown:
		// Not probed: creating the probe would create the volume if it is not
		// there. Not the new default either, for the reason the file comment
		// gives — and an engine that could not answer this is likely about to
		// fail the start on its own.
		e.systemLine(cb, fmt.Sprintf("could not tell whether this project already has a database, so it starts on %s, "+
			"which created every Docker-mode database before PostgreSQL %s became the default", legacyPostgresImage, postgresMajor))
		return legacyPostgresImage
	case volumePresent:
	}

	version, rec, err := e.volumePGVersion(ctx, c)
	if err != nil {
		e.systemLine(cb, unreadableVersionText(err))
		return legacyPostgresImage
	}
	image := imageForPGVersion(version)
	if version != postgresMajor && !rec.Noted {
		e.systemLine(cb, fmt.Sprintf("this project's database was created by PostgreSQL %s, so it keeps running on %s; new projects get PostgreSQL %s. "+
			"Resetting the project (astro local reset) deletes the database, and the next start creates it on PostgreSQL %s",
			version, image, postgresMajor, postgresMajor))
		rec.Noted = true
		if rec.CreatedAt != "" {
			savePGRecord(c.stateDir, rec)
		}
	}
	return image
}

// unreadableVersionText is the warning for an existing volume whose version
// could not be read.
func unreadableVersionText(err error) string {
	const rest = ", so it starts on %s, which created every Docker-mode database before PostgreSQL %s became the default. " +
		"If another version created it, PostgreSQL will refuse to start; resetting the project (astro local reset) deletes the database, " +
		"and the next start creates it on PostgreSQL %s"
	var pull *pullError
	if errors.As(err, &pull) {
		return fmt.Sprintf("could not pull %s to read which PostgreSQL version created this project's database (%s)"+rest,
			pull.image, pull.err, legacyPostgresImage, postgresMajor, postgresMajor)
	}
	return fmt.Sprintf("could not read which PostgreSQL version created this project's database (%s)"+rest,
		err, legacyPostgresImage, postgresMajor, postgresMajor)
}

// volumePGVersion is the version that created the project's metadata volume,
// from the state directory's record when it describes this very volume, and
// read from the volume otherwise. The record it returns is the one to update.
func (e *Engine) volumePGVersion(ctx context.Context, c pgChoice) (string, pgRecord, error) {
	volume := metadataVolume(c.composeProject)
	createdAt := e.volumeCreatedAt(ctx, c.conn, volume)
	if rec := loadPGRecord(c.stateDir); rec.matches(c.conn.bin, volume, createdAt) {
		return rec.Version, rec, nil
	}
	version, err := e.readPGVersion(ctx, c.conn, c.composeProject, c.probeImage)
	if err != nil {
		return "", pgRecord{}, err
	}
	rec := pgRecord{Engine: c.conn.bin, Volume: volume, CreatedAt: createdAt, Version: version}
	// Without a creation time there is nothing to key the record by, so none
	// is kept, and the next start reads the volume again.
	if createdAt != "" {
		savePGRecord(c.stateDir, rec)
	}
	return version, rec, nil
}

// volumeCreatedAt is when the engine says the volume was created, or "" when
// it will not say.
func (e *Engine) volumeCreatedAt(ctx context.Context, conn engineConn, volume string) string {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "volume", "inspect", "--format", "{{.CreatedAt}}", volume)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// overrideSetsPostgresImage reports whether the project's compose override
// gives the postgres service an image of its own. An override that cannot be
// read says nothing, and compose will say what is wrong with it.
func overrideSetsPostgresImage(projectPath string) bool {
	if !rt.HasComposeOverride(projectPath) {
		return false
	}
	b, err := os.ReadFile(filepath.Join(projectPath, rt.ComposeOverrideFile))
	if err != nil {
		return false
	}
	var spec struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if yaml.Unmarshal(b, &spec) != nil {
		return false
	}
	return spec.Services[postgresService].Image != ""
}

// imageForPGVersion is the image that runs a data directory PG_VERSION says
// version created.
func imageForPGVersion(version string) string {
	switch version {
	case postgresMajor:
		return postgresImage
	case legacyPostgresMajor:
		return legacyPostgresImage
	}
	// Some other version, which this runtime never created: a compose
	// override's image did, and has since been taken out. The official image
	// publishes every major under its own tag, so that tag runs it.
	return postgresRepository + ":" + version
}

// pgVersionGrammar is what PG_VERSION holds: the major alone from 10 on, and
// major.minor before that (9.6).
var pgVersionGrammar = regexp.MustCompile(`^[1-9]\d*(\.\d+)?$`)

// parsePGVersion reads a PG_VERSION file. It is checked against the grammar
// because the answer becomes an image tag, and a file that holds anything else
// is not a data directory this should guess about.
func parsePGVersion(contents string) (string, error) {
	v := strings.TrimSpace(contents)
	if !pgVersionGrammar.MatchString(v) {
		return "", fmt.Errorf("%s holds %q, which is not a PostgreSQL version", pgVersionFile, v)
	}
	return v, nil
}

// pullError is a probe image that is not on the engine and could not be
// pulled, kept apart because "could not read the version" would send someone
// looking at their database when the network is what failed.
type pullError struct {
	image string
	err   error
}

func (p *pullError) Error() string { return fmt.Sprintf("pulling %s: %s", p.image, p.err) }

func (p *pullError) Unwrap() error { return p.err }

// readPGVersion reads PG_VERSION out of the project's metadata volume.
//
// Through a container that is created and never started: the engine mounts a
// container's volumes when it creates it, so `cp` can read the file straight
// back out, without running a process or a database, and without an image
// that has anything particular in it. probeImage is the project's own, which
// this start has just built or resolved, so the probe pulls nothing a project
// on 12 would not otherwise need. The volume is mounted read-only all the same.
//
// The probe carries the compose project's label, which is how anything that
// counts a project's containers — a leak census, a person with `docker ps` —
// sees it as the project's. Compose itself does not: it lists only the
// containers it created, so `down --remove-orphans` leaves a probe in place,
// and the start, reset and clean stop remove it by name instead.
func (e *Engine) readPGVersion(ctx context.Context, conn engineConn, composeProject, probeImage string) (string, error) {
	probe := pgProbeName(composeProject)
	// One left by an interrupted start would make the create below collide.
	e.removePGProbe(ctx, conn, composeProject)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgProbeCleanupTimeout)
		defer cancel()
		e.removePGProbe(cleanup, conn, composeProject)
	}()

	// The pull is asked for separately so its failure can be named as one;
	// left to the create, it would read as the version being unreadable.
	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "image", "inspect", "--format", "{{.Id}}", probeImage); err != nil {
		if perr := e.cmd.Run(ctx, conn.env, rt.Stdio{}, conn.bin, "pull", probeImage); perr != nil {
			return "", &pullError{image: probeImage, err: perr}
		}
	}
	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "container", "create",
		"--name", probe,
		"--label", composeProjectLabel+"="+composeProject,
		"--label", "io.astronomer.docker=true",
		"--volume", metadataVolume(composeProject)+":"+pgDataDir+":ro",
		probeImage); err != nil {
		return "", fmt.Errorf("creating a container to read it through: %w", err)
	}
	archive, err := e.cmd.Output(ctx, conn.env, conn.bin, "container", "cp", probe+":"+pgDataDir+"/"+pgVersionFile, "-")
	if err != nil {
		return "", fmt.Errorf("copying %s out of the volume: %w", pgVersionFile, err)
	}
	contents, err := firstFileInTar(archive)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", pgVersionFile, err)
	}
	return parsePGVersion(contents)
}

// removePGProbe removes the project's probe container, if there is one.
// Best effort: "no such container" is the ordinary answer, and a probe that
// survives is removed by the next start or reset.
func (e *Engine) removePGProbe(ctx context.Context, conn engineConn, composeProject string) {
	//nolint:errcheck // best effort by design; see the doc comment
	_ = e.cmd.Run(ctx, conn.env, rt.Stdio{}, conn.bin, "container", "rm", "--force", pgProbeName(composeProject))
}

// errNoFileInArchive is a copy stream that held no regular file.
var errNoFileInArchive = errors.New("no file in the archive")

// firstFileInTar returns the contents of the first regular file in a tar
// stream, which is the shape `cp <container>:<file> -` writes one file in.
func firstFileInTar(archive []byte) (string, error) {
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", errNoFileInArchive
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// PG_VERSION is a few bytes. The bound keeps a file that is not one
		// from being read whole into memory.
		b, err := io.ReadAll(io.LimitReader(tr, 64))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// systemLine reports a line from the engine itself, when anyone is listening.
func (e *Engine) systemLine(cb rt.Callbacks, text string) {
	if cb.OnLine != nil {
		cb.OnLine(rt.LogLine{Component: "system", Time: e.now(), Text: text})
	}
}
