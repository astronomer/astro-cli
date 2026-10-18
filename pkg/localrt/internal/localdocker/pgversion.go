package localdocker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
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
// The version is read from the data directory itself rather than recorded
// anywhere of ours. Postgres writes the major into PG_VERSION when it creates
// the directory, so the answer is right for every volume that exists today,
// for one a compose override's image created, and for one that outlived the
// state directory, which lives in a cache anything may clear.
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

// metadataPostgresImage picks the image the project's metadata database runs:
// postgresImage for a new database, and for an existing one the major version
// it was created with.
//
// own is whether the project's metadata volume exists, asked before anything
// in this start could have created it. Only a volume that is definitely there
// is probed: creating the probe container would otherwise create the volume,
// and an engine that could not say is about to fail the start on its own.
//
// probeImage is any image the engine already has; see readPGVersion.
func (e *Engine) metadataPostgresImage(ctx context.Context, conn engineConn, composeProject, probeImage string, own volumeLookup, cb rt.Callbacks) string {
	if own != volumePresent {
		return postgresImage
	}
	version, err := e.readPGVersion(ctx, conn, composeProject, probeImage)
	if err != nil {
		// Not a reason to refuse the start: the default is right whenever the
		// database happens to be on it, and an empty volume — postgres stopped
		// before it got as far as PG_VERSION — has nothing to be wrong about.
		// When the default is wrong, what the user sees is a database that will
		// not start, and this line is the only place that can say why.
		e.systemLine(cb, fmt.Sprintf("could not read which PostgreSQL version created this project's database (%s), so it starts on %s. "+
			"If another version created it, PostgreSQL will refuse to start; resetting the project (astro local reset) deletes the database, "+
			"and the next start creates a new one", err, postgresImage))
		return postgresImage
	}
	image := imageForPGVersion(version)
	if version != postgresMajor {
		e.systemLine(cb, fmt.Sprintf("this project's database was created by PostgreSQL %s, so it keeps running on %s; new projects get PostgreSQL %s. "+
			"Resetting the project (astro local reset) deletes the database, and the next start creates it on PostgreSQL %s",
			version, image, postgresMajor, postgresMajor))
	}
	return image
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
	// override's image did. The official image publishes every major under its
	// own tag, so that tag runs it — and the override, which merges over this
	// file, decides in the end anyway.
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

// readPGVersion reads PG_VERSION out of the project's metadata volume.
//
// Through a container that is created and never started: the engine mounts a
// container's volumes when it creates it, so `cp` can read the file straight
// back out, without running a process or a database, and without an image
// that has anything particular in it. probeImage is the project's own, which
// this start has just built or resolved, so the probe pulls nothing a project
// on 12 would not otherwise need. The volume is mounted read-only all the same.
func (e *Engine) readPGVersion(ctx context.Context, conn engineConn, composeProject, probeImage string) (string, error) {
	probe := pgProbeName(composeProject)
	// One left by an interrupted start would make the create below collide.
	e.removePGProbe(ctx, conn, composeProject)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgProbeCleanupTimeout)
		defer cancel()
		e.removePGProbe(cleanup, conn, composeProject)
	}()

	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "container", "create",
		"--name", probe,
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
