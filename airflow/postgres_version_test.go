package airflow

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestMajorVersion(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    int
		wantErr bool
	}{
		{in: "15", want: 15},
		{in: "12.6", want: 12},
		{in: "17.2-bookworm", want: 17},
		{in: "15-alpine", want: 15},
		{in: "17-bookworm", want: 17},
		{in: "12.6-alpine", want: 12},
		{in: " 12\n", want: 12},
		{in: "latest", wantErr: true},
		{in: "", wantErr: true},
		{in: "sha256:abc", wantErr: true},
	} {
		got, err := majorVersion(tc.in)
		if tc.wantErr {
			assert.Error(t, err, tc.in)
			continue
		}
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}
}

func TestReadSingleFileFromTar(t *testing.T) {
	got, err := readSingleFileFromTar(tarredFile(t, "12\n"))
	require.NoError(t, err)
	assert.Equal(t, "12\n", got)

	_, err = readSingleFileFromTar(bytes.NewReader(nil))
	assert.Error(t, err)
}

func newPGTestCompose(cliClient *mocks.DockerCLIClient) *DockerCompose {
	return &DockerCompose{projectName: "testproject", cliClient: cliClient}
}

// pgVersionTar is tarredFile for use from the suite-style tests in docker_test.go.
func pgVersionTar(t *testing.T, contents string) io.ReadCloser {
	return tarredFile(t, contents)
}

func tarredFile(t *testing.T, contents string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: pgVersionFile, Mode: 0o600, Size: int64(len(contents)), Typeflag: tar.TypeReg}))
	_, err := tw.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return io.NopCloser(&buf)
}

// labelFilter matches a VolumeList call filtering on the compose labels for this
// project. Asserting the arguments is the point: the lookup has to use the same project
// name compose did, and a mock that accepts anything cannot tell when it does not.
func labelFilter(project string) any {
	return mock.MatchedBy(func(opts volume.ListOptions) bool {
		want := filters.NewArgs(
			filters.Arg("label", composeProjectLabel+"="+project),
			filters.Arg("label", composeVolumeLabel+"="+postgresDataVolume),
		)
		return reflect.DeepEqual(opts.Filters, want)
	})
}

func nameFilter(name string) any {
	return mock.MatchedBy(func(opts volume.ListOptions) bool {
		return reflect.DeepEqual(opts.Filters, filters.NewArgs(filters.Arg("name", name)))
	})
}

func TestFindPostgresDataVolume(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("finds the volume by compose labels", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("VolumeList", mock.Anything, labelFilter("testproject")).
			Return(volume.ListResponse{Volumes: []*volume.Volume{{Name: "testproject_postgres_data"}}}, nil).Once()

		got, err := newPGTestCompose(cli).findPostgresDataVolume(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "testproject_postgres_data", got)
		cli.AssertExpectations(t)
	})

	t.Run("looks up the name compose normalized, not merely lowercased", func(t *testing.T) {
		// compose builds its project name with normalizeName, which strips everything
		// outside [a-z0-9_-]. Looking up the raw name would miss the volume entirely
		// and start a new postgres on an existing data directory.
		cli := new(mocks.DockerCLIClient)
		cli.On("VolumeList", mock.Anything, labelFilter("myproj_ab12")).
			Return(volume.ListResponse{Volumes: []*volume.Volume{{Name: "myproj_ab12_postgres_data"}}}, nil).Once()

		d := &DockerCompose{projectName: "My.Proj_ab12", cliClient: cli}
		got, err := d.findPostgresDataVolume(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "myproj_ab12_postgres_data", got)
		cli.AssertExpectations(t)
	})

	t.Run("falls back to the conventional name", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("VolumeList", mock.Anything, labelFilter("testproject")).
			Return(volume.ListResponse{}, nil).Once()
		cli.On("VolumeList", mock.Anything, nameFilter("testproject_postgres_data")).
			Return(volume.ListResponse{Volumes: []*volume.Volume{{Name: "testproject_postgres_data"}}}, nil).Once()

		got, err := newPGTestCompose(cli).findPostgresDataVolume(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "testproject_postgres_data", got)
		cli.AssertExpectations(t)
	})

	t.Run("returns empty when the project has no volume", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("VolumeList", mock.Anything, mock.Anything).Return(volume.ListResponse{}, nil).Twice()

		got, err := newPGTestCompose(cli).findPostgresDataVolume(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// expectProbe sets up the volume lookup and PG_VERSION read for a project whose data
// directory was written by the given major version.
func expectProbe(t *testing.T, cli *mocks.DockerCLIClient, pgVersion string) {
	t.Helper()
	cli.On("VolumeList", mock.Anything, mock.Anything).
		Return(volume.ListResponse{Volumes: []*volume.Volume{{Name: "vol"}}}, nil).Once()
	cli.On("ImageInspect", mock.Anything, mock.Anything).Return(image.InspectResponse{}, nil).Once()
	cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
	cli.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.CreateResponse{ID: "probe-id"}, nil).Once()
	cli.On("CopyFromContainer", mock.Anything, "probe-id", pgDataDir+"/"+pgVersionFile).
		Return(tarredFile(t, pgVersion), container.PathStat{}, nil).Once()
	cli.On("ContainerRemove", mock.Anything, "probe-id", mock.Anything).Return(nil).Once()
}

func TestReadVolumePGVersion(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("reads the major version out of the volume", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("ImageInspect", mock.Anything, mock.Anything).Return(image.InspectResponse{}, nil).Once()
		cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
		cli.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(container.CreateResponse{ID: "probe-id"}, nil).Once()
		cli.On("CopyFromContainer", mock.Anything, "probe-id", pgDataDir+"/"+pgVersionFile).
			Return(tarredFile(t, "12\n"), container.PathStat{}, nil).Once()
		cli.On("ContainerRemove", mock.Anything, "probe-id", mock.Anything).Return(nil).Once()

		assert.Equal(t, 12, newPGTestCompose(cli).readVolumePGVersion(context.Background(), "vol"))
		cli.AssertExpectations(t)
	})

	t.Run("treats a missing PG_VERSION as an uninitialized volume", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("ImageInspect", mock.Anything, mock.Anything).Return(image.InspectResponse{}, nil).Once()
		cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
		cli.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(container.CreateResponse{ID: "probe-id"}, nil).Once()
		cli.On("CopyFromContainer", mock.Anything, "probe-id", pgDataDir+"/"+pgVersionFile).
			Return(io.NopCloser(bytes.NewReader(nil)), container.PathStat{}, errMockDocker).Once()
		cli.On("ContainerRemove", mock.Anything, "probe-id", mock.Anything).Return(nil).Once()

		assert.Equal(t, 0, newPGTestCompose(cli).readVolumePGVersion(context.Background(), "vol"))
	})

	t.Run("garbage in PG_VERSION is no opinion rather than a failed start", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("ImageInspect", mock.Anything, mock.Anything).Return(image.InspectResponse{}, nil).Once()
		cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
		cli.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(container.CreateResponse{ID: "probe-id"}, nil).Once()
		cli.On("CopyFromContainer", mock.Anything, "probe-id", pgDataDir+"/"+pgVersionFile).
			Return(tarredFile(t, "not a version\n"), container.PathStat{}, nil).Once()
		cli.On("ContainerRemove", mock.Anything, "probe-id", mock.Anything).Return(nil).Once()

		assert.Equal(t, 0, newPGTestCompose(cli).readVolumePGVersion(context.Background(), "vol"))
	})

	t.Run("a docker failure is no opinion rather than a failed start", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("ImageInspect", mock.Anything, mock.Anything).Return(image.InspectResponse{}, nil).Once()
		cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
		cli.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(container.CreateResponse{}, errMockDocker).Once()

		assert.Equal(t, 0, newPGTestCompose(cli).readVolumePGVersion(context.Background(), "vol"))
		cli.AssertNotCalled(t, "ContainerRemove", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestResolvePostgresTag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("a new project follows the configured tag", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("VolumeList", mock.Anything, mock.Anything).Return(volume.ListResponse{}, nil).Twice()

		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		cli.AssertExpectations(t)
	})

	t.Run("an existing project keeps the version it was created with", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		expectProbe(t, cli, "12\n")

		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "12", got, "should pin to the data directory's version, not the configured tag")
		cli.AssertExpectations(t)
	})

	t.Run("no override when the project already matches", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		expectProbe(t, cli, "15\n")

		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a newer data directory is kept too", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		expectProbe(t, cli, "17\n")

		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "17", got)
	})

	t.Run("an unreadable tag leaves the project alone", func(t *testing.T) {
		config.CFG.PostgresTag.SetHomeString("latest")
		defer config.CFG.PostgresTag.SetHomeString(config.PostgresTagDefault)

		cli := new(mocks.DockerCLIClient)
		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		cli.AssertNotCalled(t, "VolumeList", mock.Anything, mock.Anything)
	})

	t.Run("skips the check when no volume is mounted", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		config.InitConfig(fs)
		config.CFG.DuplicateImageVolumes.SetHomeString("false")
		defer config.CFG.DuplicateImageVolumes.SetHomeString("true")

		cli := new(mocks.DockerCLIClient)
		got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		cli.AssertNotCalled(t, "VolumeList", mock.Anything, mock.Anything)
	})
}

func TestResolvePostgresTagLeavesCustomRepositoriesAlone(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// Pinning asks for <repository>:<major>, which only the official image is known to
	// publish — postgis, pgvector and private mirrors often carry no such tag, so an
	// override would point the project at an image that does not exist.
	config.CFG.PostgresRepository.SetHomeString("docker.io/postgis/postgis")
	defer config.CFG.PostgresRepository.SetHomeString(config.PostgresRepositoryDefault)

	cli := new(mocks.DockerCLIClient)
	got, err := newPGTestCompose(cli).resolvePostgresTag(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
	cli.AssertNotCalled(t, "VolumeList", mock.Anything, mock.Anything)
}

func TestRemoveStaleProbes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("removes a probe left behind by an earlier run", func(t *testing.T) {
		// A stray probe holds the data volume open, which makes `astro dev kill`
		// silently fail to remove it and strands the project on its old version.
		cli := new(mocks.DockerCLIClient)
		cli.On("ContainerList", mock.Anything, mock.MatchedBy(func(opts container.ListOptions) bool {
			return opts.All && reflect.DeepEqual(opts.Filters,
				filters.NewArgs(filters.Arg("label", probeLabel+"=testproject")))
		})).Return([]container.Summary{{ID: "stale-id"}}, nil).Once()
		cli.On("ContainerRemove", mock.Anything, "stale-id", container.RemoveOptions{Force: true}).Return(nil).Once()

		newPGTestCompose(cli).removeStaleProbes(context.Background())
		cli.AssertExpectations(t)
	})

	t.Run("a listing failure is not fatal", func(t *testing.T) {
		cli := new(mocks.DockerCLIClient)
		cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, errMockDocker).Once()

		newPGTestCompose(cli).removeStaleProbes(context.Background())
		cli.AssertNotCalled(t, "ContainerRemove", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestProbeIsLabelledAndMountsTheVolume(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	cli := new(mocks.DockerCLIClient)
	cli.On("ImageInspect", mock.Anything, ImageName("testproject", "latest")).Return(image.InspectResponse{}, nil).Once()
	cli.On("ContainerList", mock.Anything, mock.Anything).Return([]container.Summary{}, nil).Once()
	cli.On("ContainerCreate", mock.Anything,
		mock.MatchedBy(func(c *container.Config) bool {
			return c.Image == ImageName("testproject", "latest") && c.Labels[probeLabel] == "testproject"
		}),
		mock.MatchedBy(func(h *container.HostConfig) bool {
			return len(h.Mounts) == 1 && h.Mounts[0].Source == "thevolume" && h.Mounts[0].Target == pgDataDir
		}),
		mock.Anything, mock.Anything, mock.Anything).
		Return(container.CreateResponse{ID: "probe-id"}, nil).Once()
	cli.On("CopyFromContainer", mock.Anything, "probe-id", pgDataDir+"/"+pgVersionFile).
		Return(tarredFile(t, "12\n"), container.PathStat{}, nil).Once()
	cli.On("ContainerRemove", mock.Anything, "probe-id", mock.Anything).Return(nil).Once()

	assert.Equal(t, 12, newPGTestCompose(cli).readVolumePGVersion(context.Background(), "thevolume"))
	cli.AssertExpectations(t)
}
