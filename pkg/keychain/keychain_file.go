//go:build !darwin

package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	// Secrets: the directory and file are owned by the user alone.
	dirPerm  = 0o700
	filePerm = 0o600
)

// insecureWarning fires the first time a plaintext write happens in a process.
// It hangs off credential writes — login and each token refresh — rather than
// store construction, because that is when secrets actually hit the disk. Once
// per process keeps it from drowning out command output while still telling the
// user on the invocation that did it.
//
// A pointer, not a value: resetting it in tests would otherwise copy a mutex.
var (
	insecureWarning              = &sync.Once{}
	insecureWarningOut io.Writer = os.Stderr
)

// warnInsecureWrite tells the user their credentials went to disk in the clear,
// so a downgrade from the OS keychain is never silent.
func warnInsecureWrite(path string) {
	insecureWarning.Do(func() {
		fmt.Fprintf(insecureWarningOut,
			"! No OS credential store available — authentication credentials saved in plain text at %s\n"+
				"  To require a secure store instead, run: astro config set -g no_insecure_fallback true\n",
			path)
	})
}

var errNoCredentialsDir = errors.New("credentials directory not configured: SetCredentialsDir must be called first")

// fileStore is a plaintext JSON credential store for environments with no
// OS-native secure store: Linux without Secret Service, and Windows, which has
// no Credential Manager backend. Credentials are written to credentials.json
// with mode 0600.
//
// Writes go via a temp-file + rename so a crash mid-write cannot corrupt an
// existing credentials file.
type fileStore struct {
	path string
}

func newFileStore() (*fileStore, error) {
	dir := CredentialsDir()
	if dir == "" {
		return nil, errNoCredentialsDir
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
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
	if err := os.Chmod(tmpPath, filePerm); err != nil {
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
	if err := s.write(store); err != nil {
		return err
	}
	warnInsecureWrite(s.path)
	return nil
}

func (s *fileStore) DeleteCredentials(domain string) error {
	store, err := s.read()
	if err != nil {
		return err
	}
	delete(store, domain)
	return s.write(store)
}
