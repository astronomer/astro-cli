package deployment

import (
	"bytes"
	httpContext "context"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestReuseBundleContent() {
	a, b, empty := testDigest("a"), testDigest("b"), testDigest("")
	plan := func(local, remote map[string]string, deleteExtras bool) *bundleSyncPlan {
		p := planBundleSync(local, remote, deleteExtras)
		reuseBundleContent(p, local, remote)
		return p
	}

	s.Run("moves a renamed file", func() {
		p := plan(map[string]string{"new.py": a}, map[string]string{"old.py": a}, true)
		s.Equal([]bundleSyncEdit{{from: "old.py", to: "new.py"}}, p.moved)
		s.Empty(p.added)
		s.Empty(p.deleted)
		s.Empty(p.copied)
	})

	s.Run("copies a duplicated file, from one it keeps even with --no-delete", func() {
		p := plan(map[string]string{"a.py": a, "b.py": a}, map[string]string{"a.py": a}, true)
		s.Equal([]bundleSyncEdit{{from: "a.py", to: "b.py"}}, p.copied)
		s.Empty(p.added)

		p = plan(map[string]string{"b.py": a}, map[string]string{"a.py": a}, false)
		s.Equal([]bundleSyncEdit{{from: "a.py", to: "b.py"}}, p.copied)
		s.Empty(p.added)
	})

	s.Run("moves a deleted file once and copies it to every other destination", func() {
		p := plan(map[string]string{"b.py": a, "c.py": a}, map[string]string{"a.py": a}, true)
		s.Equal([]bundleSyncEdit{{from: "a.py", to: "b.py"}}, p.moved)
		s.Equal([]bundleSyncEdit{{from: "a.py", to: "c.py"}}, p.copied)
		s.Empty(p.deleted)
	})

	s.Run("moves a file it would delete rather than copy one it keeps", func() {
		p := plan(map[string]string{"keep.py": a, "new.py": a}, map[string]string{"keep.py": a, "old.py": a}, true)
		s.Equal([]bundleSyncEdit{{from: "old.py", to: "new.py"}}, p.moved)
		s.Empty(p.copied)
	})

	s.Run("sends files whose content the upload changes where it is", func() {
		p := plan(map[string]string{"a.py": b, "b.py": a}, map[string]string{"a.py": a, "b.py": b}, true)
		s.Equal([]string{"a.py", "b.py"}, p.updated)
		s.Empty(p.moved)
		s.Empty(p.copied)
	})

	s.Run("sends empty files", func() {
		p := plan(map[string]string{"x/__init__.py": empty}, map[string]string{"y/__init__.py": empty}, true)
		s.Equal([]string{"x/__init__.py"}, p.added)
		s.Empty(p.moved)
	})

	s.Run("sends a file that takes the place of a directory, or of a file above it", func() {
		p := plan(map[string]string{"x/y.py": a}, map[string]string{"x": a}, true)
		s.Equal([]string{"x/y.py"}, p.added)
		s.Equal([]string{"x"}, p.deleted)
		s.Empty(p.moved)

		p = plan(map[string]string{"x": a}, map[string]string{"x/y.py": a}, true)
		s.Equal([]string{"x"}, p.added)
		s.Equal([]string{"x/y.py"}, p.deleted)
		s.Empty(p.moved)
	})
}

func (s *Suite) TestSyncBundleFilesReusesContent() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("moves a renamed file instead of sending its content again", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"renamed.py": "content"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"old.py": "content"})
		writes := make(chan map[string]string, 1)
		mockStreamClient := s.streamClient()
		mockSyncWrite(s, mockStreamClient, "v1", "v2", writes)

		err := SyncBundleFiles(httpContext.Background(), fromDir, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Equal(map[string]string{"__MV__renamed.py": "old.py"}, receive(s, writes))
		s.Contains(out.String(), "move    my-files:dags/old.py -> my-files:dags/renamed.py\n")
		s.Contains(out.String(), "Synced "+fromDir+" to my-files:dags at version v2: 0 added, 0 updated, 0 deleted, 1 moved, 0 copied")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}
