//go:build !darwin

package keychain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// fileStore is a plaintext JSON credential store for environments where no
// OS-native secure store is available (Linux without Secret Service, Windows
// before the Credential Manager backend lands). Credentials are written to
// ~/.astro/credentials.json with mode 0600.
//
// Writes go via a temp-file + rename so a crash mid-write cannot corrupt an
// existing credentials file.
type fileStore struct {
	path string
}

func newFileStore() (*fileStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".astro")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create credentials directory: %w", err)
	}
	return &fileStore{path: filepath.Join(dir, "credentials.json")}, nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *fileStore) read() (map[string]Credentials, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]Credentials{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}
	var store map[string]Credentials
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("decoding credentials file: %w", err)
	}
	return store, nil
}

func (s *fileStore) write(store map[string]Credentials) error {
	data, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	return writeAtomic(s.path, data)
}

func (s *fileStore) GetCredentials(domain string) (Credentials, error) {
	store, err := s.read()
	if err != nil {
		return Credentials{}, err
	}
	creds, ok := store[domain]
	if !ok {
		return Credentials{}, ErrNotFound
	}
	return creds, nil
}

func (s *fileStore) SetCredentials(domain string, creds Credentials) error {
	store, err := s.read()
	if err != nil {
		return err
	}
	store[domain] = creds
	return s.write(store)
}

func (s *fileStore) DeleteCredentials(domain string) error {
	store, err := s.read()
	if err != nil {
		return err
	}
	delete(store, domain)
	return s.write(store)
}
