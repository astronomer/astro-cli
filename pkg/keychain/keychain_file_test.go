//go:build !darwin

package keychain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_CRUD(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, s *fileStore)
	}{
		{
			name: "get from missing file returns ErrNotFound",
			run: func(t *testing.T, s *fileStore) {
				_, err := s.GetCredentials("example.com")
				assert.ErrorIs(t, err, ErrNotFound)
			},
		},
		{
			name: "set and get round-trip",
			run: func(t *testing.T, s *fileStore) {
				creds := Credentials{Token: "tok", UserEmail: "a@b.com"}
				require.NoError(t, s.SetCredentials("example.com", creds))

				got, err := s.GetCredentials("example.com")
				require.NoError(t, err)
				assert.Equal(t, creds, got)
			},
		},
		{
			name: "get missing domain returns ErrNotFound",
			run: func(t *testing.T, s *fileStore) {
				require.NoError(t, s.SetCredentials("a.io", Credentials{Token: "a"}))

				_, err := s.GetCredentials("b.io")
				assert.ErrorIs(t, err, ErrNotFound)
			},
		},
		{
			name: "set overwrites existing",
			run: func(t *testing.T, s *fileStore) {
				require.NoError(t, s.SetCredentials("x.io", Credentials{Token: "old"}))
				require.NoError(t, s.SetCredentials("x.io", Credentials{Token: "new"}))

				got, err := s.GetCredentials("x.io")
				require.NoError(t, err)
				assert.Equal(t, "new", got.Token)
			},
		},
		{
			name: "delete then get returns ErrNotFound",
			run: func(t *testing.T, s *fileStore) {
				require.NoError(t, s.SetCredentials("x.io", Credentials{Token: "tok"}))
				require.NoError(t, s.DeleteCredentials("x.io"))

				_, err := s.GetCredentials("x.io")
				assert.ErrorIs(t, err, ErrNotFound)
			},
		},
		{
			name: "delete preserves other domains",
			run: func(t *testing.T, s *fileStore) {
				require.NoError(t, s.SetCredentials("a.io", Credentials{Token: "a"}))
				require.NoError(t, s.SetCredentials("b.io", Credentials{Token: "b"}))
				require.NoError(t, s.DeleteCredentials("a.io"))

				got, err := s.GetCredentials("b.io")
				require.NoError(t, err)
				assert.Equal(t, "b", got.Token)
			},
		},
		{
			name: "file has 0600 permissions after set",
			run: func(t *testing.T, s *fileStore) {
				require.NoError(t, s.SetCredentials("x.io", Credentials{Token: "tok"}))

				info, err := os.Stat(s.path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &fileStore{path: filepath.Join(t.TempDir(), "credentials.json")}
			tt.run(t, s)
		})
	}
}

func TestFileStore_CorruptJSON(t *testing.T) {
	s := &fileStore{path: filepath.Join(t.TempDir(), "credentials.json")}
	require.NoError(t, os.WriteFile(s.path, []byte("not json{{{"), 0o600))

	_, err := s.GetCredentials("example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding credentials file")
}

func TestWriteAtomic(t *testing.T) {
	t.Run("writes file with expected content and permissions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.json")
		require.NoError(t, writeAtomic(path, []byte(`{"key":"value"}`)))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, `{"key":"value"}`, string(data))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("error when target dir does not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nodir", "file.json")
		err := writeAtomic(path, []byte("data"))
		require.Error(t, err)
	})
}
