package airflow

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
)

const (
	// composeProjectLabel and composeVolumeLabel are set by docker compose on every
	// volume it creates, and are how we find the data volume without reimplementing
	// compose's project name normalization.
	composeProjectLabel = "com.docker.compose.project"
	composeVolumeLabel  = "com.docker.compose.volume"

	// postgresDataVolume is the volume key declared in the compose templates.
	postgresDataVolume = "postgres_data"

	// pgDataDir is where the postgres image keeps its data directory, and PG_VERSION
	// is the file in it recording the major version that wrote it.
	pgDataDir     = "/var/lib/postgresql/data"
	pgVersionFile = "PG_VERSION"

	// probeLabel marks the throwaway container used to read the data directory, so one
	// left behind by an interrupted start can be found and removed. It carries a label
	// rather than a name because a leftover name would collide, and a failed create
	// would silently switch the version check off.
	probeLabel = "io.astronomer.astro.postgres-version-probe"
)

// resolvePostgresTag decides which postgres image a project starts with, returning an
// empty string to mean "whatever postgres.tag is set to".
//
// A project that already has a data directory keeps the major version that wrote it,
// because a newer server refuses to read an older data directory and a bumped default
// must not strand a working project. Projects with no data directory — new ones, and
// any project after `astro dev kill` — follow the configured tag.
//
// Moving an existing project onto a new version is therefore `astro dev kill` followed
// by `astro dev start`, which discards the local database along with the volume.
func (d *DockerCompose) resolvePostgresTag(ctx context.Context) (string, error) {
	if !config.CFG.DuplicateImageVolumes.GetBool() {
		// The compose template mounts no volume in this mode, so there is no data
		// directory to outlive the container.
		return "", nil
	}

	// Pinning means asking for <repository>:<major>, which only the official postgres
	// image is guaranteed to publish. A custom repository is the user's to version.
	if repo := config.CFG.PostgresRepository.GetString(); repo != config.PostgresRepositoryDefault {
		logger.Debugf("skipping postgres version check: postgres.repository is %s", repo)
		return "", nil
	}

	configuredMajor, err := majorVersion(config.CFG.PostgresTag.GetString())
	if err != nil {
		// An unparseable tag is the user's own pin — a digest, "latest", an internal
		// build. There is nothing to compare against, so let compose start it.
		logger.Debugf("skipping postgres version check: %s", err)
		return "", nil
	}

	volName, err := d.findPostgresDataVolume(ctx)
	if err != nil || volName == "" {
		return "", err
	}

	// A version that cannot be read is not a reason to refuse to start: fall back to
	// the configured tag, which is what a project with no data directory gets anyway.
	diskMajor := d.readVolumePGVersion(ctx, volName)
	if diskMajor == 0 || diskMajor == configuredMajor {
		return "", nil
	}

	logger.Debugf("using postgres %d for this project, which is what %s was created with (postgres.tag is %s)",
		diskMajor, volName, config.CFG.PostgresTag.GetString())

	return strconv.Itoa(diskMajor), nil
}

// findPostgresDataVolume locates this project's postgres volume by the labels compose
// stamps on it, falling back to the name compose derives from the project name.
func (d *DockerCompose) findPostgresDataVolume(ctx context.Context) (string, error) {
	// normalizeName is what compose was given as the project name, so it is what the
	// labels hold — lowercasing alone would miss any name carrying a dot, a space or
	// a non-ascii character.
	res, err := d.cliClient.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs(
		filters.Arg("label", composeProjectLabel+"="+normalizeName(d.projectName)),
		filters.Arg("label", composeVolumeLabel+"="+postgresDataVolume),
	)})
	if err != nil {
		return "", fmt.Errorf("error looking up the postgres data volume: %w", err)
	}
	if len(res.Volumes) > 0 {
		return res.Volumes[0].Name, nil
	}

	fallback := normalizeName(d.projectName) + "_" + postgresDataVolume
	exists, err := d.volumeExists(ctx, fallback)
	if err != nil || !exists {
		return "", err
	}
	return fallback, nil
}

func (d *DockerCompose) volumeExists(ctx context.Context, name string) (bool, error) {
	res, err := d.cliClient.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs(filters.Arg("name", name))})
	if err != nil {
		return false, fmt.Errorf("error listing volumes: %w", err)
	}
	for _, v := range res.Volumes {
		if v.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// readVolumePGVersion reports the postgres major version recorded in the volume, or 0
// when it cannot be read — an empty data directory, or a docker call that did not work.
// The container is created but never started: docker populates volume mounts at
// creation, so the file can be copied straight back out for a fraction of the cost of
// starting a container.
func (d *DockerCompose) readVolumePGVersion(ctx context.Context, volName string) int {
	img, err := d.probeImage(ctx)
	if err != nil {
		logger.Debugf("skipping postgres version check: %s", err)
		return 0
	}

	// A probe from an interrupted start would still be holding the volume, which stops
	// `astro dev kill` removing it — and a project whose volume cannot be dropped can
	// never move to a new postgres version. Clear any out before adding another.
	d.removeStaleProbes(ctx)

	created, err := d.cliClient.ContainerCreate(ctx,
		&container.Config{Image: img, Entrypoint: []string{"true"}, Labels: map[string]string{probeLabel: d.projectName}},
		&container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: volName, Target: pgDataDir}}},
		nil, nil, "")
	if err != nil {
		logger.Debugf("skipping postgres version check, could not create the probe container: %s", err)
		return 0
	}
	defer func() {
		if rErr := d.cliClient.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true}); rErr != nil {
			logger.Debugf("error removing postgres probe container: %s", rErr)
		}
	}()

	rc, _, err := d.cliClient.CopyFromContainer(ctx, created.ID, pgDataDir+"/"+pgVersionFile)
	if err != nil {
		// Either way the configured tag is used, so a docker failure here can still
		// put a new postgres on an old data directory. The two are logged apart
		// because only the second is worth investigating.
		if errdefs.IsNotFound(err) {
			logger.Debugf("no %s in volume %s", pgVersionFile, volName)
		} else {
			logger.Debugf("could not read %s from volume %s: %s", pgVersionFile, volName, err)
		}
		return 0
	}
	defer rc.Close()

	contents, err := readSingleFileFromTar(rc)
	if err != nil {
		logger.Debugf("could not read %s from volume %s: %s", pgVersionFile, volName, err)
		return 0
	}

	major, err := majorVersion(contents)
	if err != nil {
		logger.Debugf("unreadable %s in volume %s: %s", pgVersionFile, volName, err)
		return 0
	}
	return major
}

// removeStaleProbes deletes probe containers left behind by an earlier run. They are
// found by label: naming them instead would mean a leftover collided with the next
// create, which would silently switch the version check off.
func (d *DockerCompose) removeStaleProbes(ctx context.Context) {
	existing, err := d.cliClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", probeLabel+"="+d.projectName)),
	})
	if err != nil {
		logger.Debugf("could not list stale postgres probe containers: %s", err)
		return
	}
	for i := range existing {
		if rErr := d.cliClient.ContainerRemove(ctx, existing[i].ID, container.RemoveOptions{Force: true}); rErr != nil {
			logger.Debugf("could not remove stale postgres probe container %s: %s", existing[i].ID, rErr)
		}
	}
}

// probeImage picks an image to mount the data volume into. The container is never
// started, so any image will do — preferring the project's own, which was just built,
// keeps the check from pulling a postgres image the project may not even use.
func (d *DockerCompose) probeImage(ctx context.Context) (string, error) {
	projectImage := ImageName(d.projectName, "latest")
	if _, err := d.cliClient.ImageInspect(ctx, projectImage); err == nil {
		return projectImage, nil
	}

	img := config.CFG.PostgresRepository.GetString() + ":" + config.CFG.PostgresTag.GetString()
	if _, err := d.cliClient.ImageInspect(ctx, img); err == nil {
		return img, nil
	}

	rc, err := d.cliClient.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("error pulling %s: %w", img, err)
	}
	defer rc.Close()

	// The pull finishes only once its response body has been consumed.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return "", fmt.Errorf("error pulling %s: %w", img, err)
	}
	return img, nil
}

// majorVersion parses the leading major version out of a postgres tag or a PG_VERSION
// file.
func majorVersion(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("empty postgres version")
	}
	head := v
	if i := strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		head = v[:i]
	}
	n, err := strconv.Atoi(head)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("cannot read a major version from %q", v)
	}
	return n, nil
}

// readSingleFileFromTar reads the first regular file out of a docker copy stream.
func readSingleFileFromTar(r io.Reader) (string, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("no file in archive")
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
