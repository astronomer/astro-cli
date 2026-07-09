package cosmosboost

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/config"
)

const (
	// MinVersion is the minimum astro-cosmos-boost version compatible with
	// this CLI. Bump when the CLI starts depending on newer helper behavior.
	MinVersion = "0.0.2-rc.1"

	// defaultBaseURL is the production install CDN for the helper binary.
	defaultBaseURL = "https://install.astronomer.io/astro-cosmos-boost"

	binaryName  = "astro-cosmos-boost"
	windowsGOOS = "windows"
	dirPerm     = 0o755
	binPerm     = 0o755
)

// BaseURL returns the CDN base the helper is downloaded from. The
// cosmos_boost.base_url config overrides the production default so the
// integration can be exercised against another bucket end-to-end.
func BaseURL() string {
	if url := config.CFG.CosmosBoostBaseURL.GetString(); url != "" {
		return strings.TrimSuffix(url, "/")
	}
	return defaultBaseURL
}

// BinDir returns the directory where the helper is installed (~/.astro/bin/).
func BinDir() string {
	return filepath.Join(config.HomeConfigPath, "bin")
}

// BinaryPath returns the full path to the helper binary.
func BinaryPath() string {
	name := binaryName
	if runtime.GOOS == windowsGOOS {
		name += ".exe"
	}
	return filepath.Join(BinDir(), name)
}

// InstalledVersion asks the installed helper for its version. It returns ""
// when the binary is missing or won't run — both mean "reinstall", which
// EnsureBinary does.
func InstalledVersion() string {
	if _, err := os.Stat(BinaryPath()); err != nil {
		return ""
	}
	out, err := exec.Command(BinaryPath(), "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// LatestVersion fetches the latest helper version from the CDN.
func LatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(BaseURL() + "/latest/version")
	if err != nil {
		return "", fmt.Errorf("checking latest astro-cosmos-boost version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checking latest astro-cosmos-boost version: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading latest astro-cosmos-boost version: %w", err)
	}
	return strings.TrimSpace(string(body)), nil
}

// meetsMinVersion reports whether an installed version string satisfies
// MinVersion. Empty or unparseable versions do not.
func meetsMinVersion(installed string) bool {
	if installed == "" {
		return false
	}
	iv, err := semver.NewVersion(installed)
	if err != nil {
		return false
	}
	minVer, _ := semver.NewVersion(MinVersion)
	return !iv.LessThan(minVer)
}

// EnsureBinary downloads the helper if it's missing or below MinVersion, and
// re-checks the gate after installing: if the CDN's latest release is itself
// below MinVersion (e.g. mid-rollback), succeeding silently would defeat the
// version gate, so that is an error.
func EnsureBinary() error {
	installed := InstalledVersion()
	if meetsMinVersion(installed) {
		return nil
	}
	if installed == "" {
		fmt.Println("Downloading astro-cosmos-boost...")
	} else {
		fmt.Printf("astro-cosmos-boost %s is below minimum required version %s, updating...\n", installed, MinVersion)
	}
	if err := downloadAndInstall(); err != nil {
		return err
	}
	if v := InstalledVersion(); !meetsMinVersion(v) {
		return fmt.Errorf("installed astro-cosmos-boost %s is still below minimum required version %s (the CDN may be serving an older release)", v, MinVersion)
	}
	return nil
}

// archiveName returns the release archive filename for this platform,
// matching the helper's GoReleaser name_template
// ({ProjectName}_{Version}_{Os}_{Arch}, zip on Windows).
func archiveName(version string) string {
	ext := "tar.gz"
	if runtime.GOOS == windowsGOOS {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", binaryName, version, runtime.GOOS, runtime.GOARCH, ext)
}

// downloadAndInstall fetches the latest release archive from the immutable
// versioned CDN path, verifies it against the release's SHA256SUMS, and
// installs the binary into BinDir. Downloading from the versioned directory
// (not latest/) means the archive and checksums can't race a concurrent
// release overwriting latest/.
func downloadAndInstall() error {
	version, err := LatestVersion()
	if err != nil {
		return err
	}

	base := fmt.Sprintf("%s/v%s", BaseURL(), version)
	archive := archiveName(version)

	archivePath, err := downloadToTemp(base + "/" + archive)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := verifyChecksum(archivePath, archive, fmt.Sprintf("%s/%s_%s_SHA256SUMS", base, binaryName, version)); err != nil {
		return err
	}

	if err := extractBinary(archivePath); err != nil {
		return err
	}

	if v := InstalledVersion(); v != "" {
		fmt.Printf("astro-cosmos-boost %s installed\n", v)
	}
	return nil
}

// downloadToTemp fetches url into a temp file and returns its path.
func downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading astro-cosmos-boost: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading astro-cosmos-boost: HTTP %d from %s", resp.StatusCode, url)
	}

	tmpFile, err := os.CreateTemp("", "cosmosboost-download-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("downloading astro-cosmos-boost: %w", err)
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}

// verifyChecksum downloads the release's SHA256SUMS from sumsURL and checks
// that the sha256 of the file at path matches the entry for archive. Unlike
// the otto downloader, a mismatch is fatal: the helper stamps files that are
// shipped to a deployment, so we refuse to run a binary that doesn't match
// its published checksum.
func verifyChecksum(path, archive, sumsURL string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sumsURL)
	if err != nil {
		return fmt.Errorf("downloading astro-cosmos-boost checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading astro-cosmos-boost checksums: HTTP %d from %s", resp.StatusCode, sumsURL)
	}
	sums, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading astro-cosmos-boost checksums: %w", err)
	}

	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archive {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s in %s", archive, sumsURL)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening downloaded archive: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing downloaded archive: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", archive, got, want)
	}
	return nil
}

// extractBinary pulls the helper binary out of the (already verified)
// archive at archivePath and installs it at BinaryPath. The binary is
// written to a temp name and renamed into place, so a concurrent invocation
// never sees a half-written executable.
func extractBinary(archivePath string) error {
	binDir := BinDir()
	if err := os.MkdirAll(binDir, dirPerm); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	wantName := binaryName
	if runtime.GOOS == windowsGOOS {
		wantName += ".exe"
	}

	var reader io.ReadCloser
	var err error
	if runtime.GOOS == windowsGOOS {
		reader, err = zipMember(archivePath, wantName)
	} else {
		reader, err = tarGzMember(archivePath, wantName)
	}
	if err != nil {
		return err
	}
	defer reader.Close()

	tmpBin, err := os.CreateTemp(binDir, ".cosmosboost-*")
	if err != nil {
		return fmt.Errorf("creating temp binary: %w", err)
	}
	defer os.Remove(tmpBin.Name())
	if _, err := io.Copy(tmpBin, reader); err != nil {
		tmpBin.Close()
		return fmt.Errorf("extracting astro-cosmos-boost: %w", err)
	}
	tmpBin.Close()

	if err := os.Chmod(tmpBin.Name(), binPerm); err != nil {
		return fmt.Errorf("marking astro-cosmos-boost executable: %w", err)
	}
	if err := os.Rename(tmpBin.Name(), BinaryPath()); err != nil {
		return fmt.Errorf("installing astro-cosmos-boost: %w", err)
	}
	return nil
}

// tarGzMember returns a reader for the named member of a .tar.gz archive.
// The caller must Close it.
func tarGzMember(archivePath, name string) (io.ReadCloser, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("opening archive: %w", err)
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == name {
			return &memberReader{Reader: tr, closers: []io.Closer{gz, f}}, nil
		}
	}
	f.Close()
	return nil, fmt.Errorf("binary %s not found in archive", name)
}

// zipMember returns a reader for the named member of a .zip archive.
// The caller must Close it.
func zipMember(archivePath, name string) (io.ReadCloser, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("opening archive: %w", err)
	}
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) == name && !zf.FileInfo().IsDir() {
			rc, err := zf.Open()
			if err != nil {
				zr.Close()
				return nil, fmt.Errorf("reading archive: %w", err)
			}
			return &memberReader{Reader: rc, closers: []io.Closer{rc, zr}}, nil
		}
	}
	zr.Close()
	return nil, fmt.Errorf("binary %s not found in archive", name)
}

// memberReader wraps an archive member's reader together with the underlying
// resources that must be closed when the member has been consumed.
type memberReader struct {
	io.Reader
	closers []io.Closer
}

func (m *memberReader) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
