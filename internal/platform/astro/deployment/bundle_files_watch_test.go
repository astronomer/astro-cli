package deployment

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestSyncBundleFilesWatchRefusesArguments() {
	out := &bytes.Buffer{}
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

	err := SyncBundleFiles(httpContext.Background(), "my-files:a", "./b", &BundleSyncOptions{Watch: true}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncWatch)

	err = SyncBundleFiles(httpContext.Background(), "./a", "my-files:b", &BundleSyncOptions{Watch: true, DryRun: true}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncWatchDryRun)
}

// watchOutput collects the output of a watching sync, and closes watching when
// the sync says it is watching.
type watchOutput struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	watching chan struct{}
}

func (w *watchOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if bytes.HasPrefix(p, []byte("Watching ")) {
		close(w.watching)
	}
	return w.buf.Write(p)
}

func (w *watchOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// waitFor waits for the output to hold want, failing the test if it does not in time.
func (w *watchOutput) waitFor(s *Suite, want string) {
	s.Eventually(func() bool { return strings.Contains(w.String(), want) }, 5*time.Second, 10*time.Millisecond, want)
}

// receive returns the next value from c, failing the test if none arrives in time.
func receive[T any](s *Suite, c <-chan T) T {
	select {
	case v := <-c:
		return v
	case <-time.After(5 * time.Second):
		s.FailNow("timed out waiting for the watching sync")
		var zero T
		return zero
	}
}

// mockSyncWrite mocks one write into dags that expects version and creates
// newVersion, and sends the files it wrote to writes.
func mockSyncWrite(s *Suite, mockStreamClient *astrov1alpha1_mocks.ClientInterface, version, newVersion string, writes chan<- map[string]string) {
	mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
		return *p.Path == "dags" && *p.ExpectedVersion == version
	}), bundleFileContentType, mock.Anything).Run(func(args mock.Arguments) {
		files, err := bundleDirectoryFiles(args.Get(6).(io.Reader))
		s.NoError(err)
		writes <- files
	}).Return(directoryWritten(newVersion), nil).Once()
}

// replaceLocalFiles writes each file under dir in one rename, as an editor that
// saves atomically does, so a sync running meanwhile never reads one half written.
func replaceLocalFiles(s *Suite, dir string, files map[string]string) {
	for name, content := range files {
		filePath := filepath.Join(dir, filepath.FromSlash(name))
		s.NoError(os.MkdirAll(filepath.Dir(filePath), 0o755))
		temp := filepath.Join(s.T().TempDir(), filepath.Base(filePath))
		s.NoError(os.WriteFile(temp, []byte(content), 0o600))
		s.NoError(os.Rename(temp, filePath))
	}
}

// startWatch runs a watching sync of fromDir into my-files:dags until stop is
// called or the test ends.
func startWatch(s *Suite, fromDir string, mockV1Alpha1Client *astrov1alpha1_mocks.ClientWithResponsesInterface, mockStreamClient *astrov1alpha1_mocks.ClientInterface) (out *watchOutput, done <-chan error, stop func()) {
	ctx, cancel := httpContext.WithCancel(httpContext.Background())
	s.T().Cleanup(cancel)
	out = &watchOutput{watching: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		result <- SyncBundleFiles(ctx, fromDir, "my-files:dags", &BundleSyncOptions{Watch: true}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, mockStreamClient)
	}()
	return out, result, cancel
}

func (s *Suite) TestSyncBundleFilesWatch() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	quietPeriod, maxDelay, retryDelay := bundleSyncQuietPeriod, bundleSyncMaxDelay, bundleSyncRetryDelay
	bundleSyncQuietPeriod, bundleSyncMaxDelay, bundleSyncRetryDelay = 200*time.Millisecond, 600*time.Millisecond, 200*time.Millisecond
	s.T().Cleanup(func() {
		bundleSyncQuietPeriod, bundleSyncMaxDelay, bundleSyncRetryDelay = quietPeriod, maxDelay, retryDelay
	})

	s.Run("syncs each change at the version it last wrote, without listing again", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})
		writes := make(chan map[string]string, 3)
		mockStreamClient := s.streamClient()
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)
		mockSyncWrite(s, mockStreamClient, "v2", "v3", writes)
		mockSyncWrite(s, mockStreamClient, "v3", "v4", writes)

		out, done, stop := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		s.Equal(map[string]string{"a.py": "a"}, receive(s, writes))
		receive(s, out.watching)

		replaceLocalFiles(s, fromDir, map[string]string{"sub/b.py": "b"})
		s.NoError(os.Remove(filepath.Join(fromDir, "a.py")))
		s.Equal(map[string]string{"sub/b.py": "b", "__DEL__a.py": ""}, receive(s, writes))

		replaceLocalFiles(s, fromDir, map[string]string{"sub/b.py": "b2", ".env": "secret"})
		s.Equal(map[string]string{"sub/b.py": "b2"}, receive(s, writes))

		stop()
		s.NoError(receive(s, done))
		s.Contains(out.String(), "Synced "+fromDir+" to my-files:dags at version v3: 1 added, 0 updated, 1 deleted")
		s.Contains(out.String(), "update  my-files:dags/sub/b.py\nSynced "+fromDir+" to my-files:dags at version v4: 0 added, 1 updated, 0 deleted")
		s.Contains(out.String(), "Stopped watching "+fromDir)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("syncs a change in a subdirectory that existed before the watch, from a relative SRC", func() {
		root := s.T().TempDir()
		s.T().Chdir(root)
		files := map[string]string{"a.py": "a", "sub/b.py": "b", "deep/er/c.py": "c"}
		writeLocalFiles(s, filepath.Join(root, "src"), files)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", files)
		writes := make(chan map[string]string, 2)
		mockStreamClient := s.streamClient()
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)
		mockSyncWrite(s, mockStreamClient, "v2", "v3", writes)

		out, done, stop := startWatch(s, "./src", mockV1Alpha1Client, mockStreamClient)
		receive(s, out.watching)

		replaceLocalFiles(s, "src", map[string]string{"deep/er/c.py": "c2"})
		s.Equal(map[string]string{"deep/er/c.py": "c2"}, receive(s, writes))

		s.NoError(os.Remove(filepath.Join("src", "sub", "b.py")))
		s.Equal(map[string]string{"__DEL__sub/b.py": ""}, receive(s, writes))

		stop()
		s.NoError(receive(s, done))
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("reports a file that breaks a limit and keeps watching until it is fixed", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", "bad name.py": "x"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})
		writes := make(chan map[string]string, 1)
		mockStreamClient := s.streamClient()
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)

		out, done, stop := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		out.waitFor(s, "nothing was written, because these break the bundle limits:\n  "+filepath.Join(fromDir, "bad name.py")+": its bundle path dags/bad name.py may hold only")
		s.NoError(os.Remove(filepath.Join(fromDir, "bad name.py")))
		s.Equal(map[string]string{"a.py": "a"}, receive(s, writes))

		stop()
		s.NoError(receive(s, done))
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("tries a write that failed again, at the same version", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return *p.ExpectedVersion == "v1"
		}), bundleFileContentType, mock.Anything).Return(streamedResponse(http.StatusServiceUnavailable, []byte(`{"message":"try again later"}`)), nil).Once()
		writes := make(chan map[string]string, 1)
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)

		out, done, stop := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		s.Equal(map[string]string{"a.py": "a"}, receive(s, writes))

		stop()
		s.NoError(receive(s, done))
		s.Contains(out.String(), "failed to sync "+fromDir+" (try again later); retrying in 200ms\n")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("applies a .gitignore changed during the watch, keeping the files it newly skips", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", "b.log": "b"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})
		mockStreamClient := s.streamClient()
		writes := make(chan map[string]string, 2)
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)
		mockSyncWrite(s, mockStreamClient, "v2", "v3", writes)

		_, done, stop := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		s.Equal(map[string]string{"a.py": "a", "b.log": "b"}, receive(s, writes))

		replaceLocalFiles(s, fromDir, map[string]string{".gitignore": "*.log\n", "b.log": "b2"})
		s.Equal(map[string]string{".gitignore": "*.log\n"}, receive(s, writes))

		stop()
		s.NoError(receive(s, done))
	})

	s.Run("syncs a file that keeps changing once the longest delay passes", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"log.txt": "0"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"log.txt": "0"})
		writes := make(chan map[string]string, 1)
		mockStreamClient := s.streamClient()
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)

		out, done, stop := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		receive(s, out.watching)
		// Each change comes well inside the quiet period, so only the longest delay lets a sync run.
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(5 * time.Second)
		for i := 1; ; i++ {
			select {
			case files := <-writes:
				s.Contains(files, "log.txt")
				stop()
				s.NoError(receive(s, done))
				mockV1Alpha1Client.AssertExpectations(s.T())
				return
			case <-ticker.C:
				replaceLocalFiles(s, fromDir, map[string]string{"log.txt": strconv.Itoa(i)})
			case <-timeout:
				s.FailNow("timed out waiting for a sync while the file kept changing")
			}
		}
	})

	s.Run("refuses to watch more directories than it allows", func() {
		fromDir := s.T().TempDir()
		for i := range maxWatchedDirs {
			s.NoError(os.Mkdir(filepath.Join(fromDir, strconv.Itoa(i)), 0o755))
		}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})

		mockStreamClient := s.streamClient()
		_, done, _ := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		s.EqualError(receive(s, done), "failed to watch "+fromDir+": --watch watches at most 1024 directories; skip some with --exclude")
	})

	s.Run("stops when something else changed the bundle", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"a.py": "a"})
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return *p.ExpectedVersion == "v1"
		}), bundleFileContentType, mock.Anything).Return(streamedResponse(http.StatusConflict, []byte(`{"message":"bundle has moved on; re-read and retry"}`)), nil).Once()

		out, done, _ := startWatch(s, fromDir, mockV1Alpha1Client, mockStreamClient)
		receive(s, out.watching)
		replaceLocalFiles(s, fromDir, map[string]string{"a.py": "a2"})
		s.EqualError(receive(s, done), "bundle has moved on; re-read and retry")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}

// Under --output json each sync a watch makes is one line of a stream on the
// result, and the notes (that it is watching, that it stopped) are on stderr.
func (s *Suite) TestSyncBundleFilesWatchPublishesAStream() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	quietPeriod, maxDelay := bundleSyncQuietPeriod, bundleSyncMaxDelay
	bundleSyncQuietPeriod, bundleSyncMaxDelay = 200*time.Millisecond, 600*time.Millisecond
	s.T().Cleanup(func() { bundleSyncQuietPeriod, bundleSyncMaxDelay = quietPeriod, maxDelay })

	fromDir := s.T().TempDir()
	writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
	mockV1Alpha1Client := s.mockBundleLookup()
	mockSyncListing(mockV1Alpha1Client, "", map[string]string{})
	writes := make(chan map[string]string, 2)
	mockStreamClient := s.streamClient()
	mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)
	mockSyncWrite(s, mockStreamClient, "v2", "v3", writes)

	result := &watchOutput{watching: make(chan struct{})}
	notes := &watchOutput{watching: make(chan struct{})}
	ctx, stop := httpContext.WithCancel(httpContext.Background())
	s.T().Cleanup(stop)
	done := make(chan error, 1)
	go func() {
		out := BundleOutput{Result: testUtil.Renderer{JSON: true, Out: result}, Content: result, Notes: notes}
		done <- SyncBundleFiles(ctx, fromDir, "my-files:dags", &BundleSyncOptions{Watch: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
	}()
	s.Equal(map[string]string{"a.py": "a"}, receive(s, writes))
	receive(s, notes.watching)

	replaceLocalFiles(s, fromDir, map[string]string{"b.py": "b"})
	s.Equal(map[string]string{"b.py": "b"}, receive(s, writes))
	result.waitFor(s, `"version":"v3"`)

	stop()
	s.NoError(receive(s, done))

	var rounds []BundleSyncResult
	for _, line := range strings.Split(strings.TrimSpace(result.String()), "\n") {
		var round BundleSyncResult
		s.Require().NoError(json.Unmarshal([]byte(line), &round), line)
		rounds = append(rounds, round)
	}
	s.Require().Len(rounds, 2)
	s.Equal(BundleSyncResult{Source: fromDir, Destination: "my-files:dags", Direction: "upload", Changed: true, Version: "v2", Added: []string{"a.py"}, Updated: []string{}, Deleted: []string{}}, rounds[0])
	s.Equal([]string{"b.py"}, rounds[1].Added)
	s.Contains(notes.String(), "Watching "+fromDir)
	s.Contains(notes.String(), "Stopped watching "+fromDir)
	s.NotContains(result.String(), "Watching")
}
