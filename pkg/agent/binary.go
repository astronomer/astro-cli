package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/config"
)

const (
	// MinVersion is the minimum Otto version compatible with this CLI.
	// Bump this when breaking changes are made to the RPC/flag interface.
	MinVersion = "0.0.2"

	cdnBaseURL    = "https://install.astronomer.io/otto"
	binaryName    = "otto"
	pkgJSON       = "package.json"
	windowsGOOS   = "windows"
	dirPerm       = 0o755
	binPerm       = 0o755
	logFilePerm   = 0o644
	stateFilePerm = 0o600
)

// packageInfo represents the package.json next to the Otto binary.
type packageInfo struct {
	Version string `json:"version"`
}

// BinDir returns the directory where Otto is installed (~/.astro/bin/).
func BinDir() string {
	return filepath.Join(config.HomeConfigPath, "bin")
}

// BinaryPath returns the full path to the Otto binary.
func BinaryPath() string {
	name := binaryName
	if runtime.GOOS == windowsGOOS {
		name += ".exe"
	}
	return filepath.Join(BinDir(), name)
}

// InstalledVersion reads the version from package.json next to the binary.
// Returns empty string if not installed.
func InstalledVersion() (string, error) {
	data, err := os.ReadFile(filepath.Join(BinDir(), pkgJSON))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading otto metadata: %w", err)
	}
	var info packageInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("parsing otto metadata: %w", err)
	}
	return info.Version, nil
}

// LatestVersion fetches the latest version string from the CDN.
func LatestVersion() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cdnBaseURL + "/latest/version")
	if err != nil {
		return "", fmt.Errorf("checking latest otto version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checking latest otto version: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading latest otto version: %w", err)
	}
	return strings.TrimSpace(string(body)), nil
}

// IsUpdateAvailable returns true if a newer version is available on the CDN.
func IsUpdateAvailable() (available bool, latest string, err error) {
	installed, err := InstalledVersion()
	if err != nil || installed == "" {
		return false, "", err
	}
	latest, err = LatestVersion()
	if err != nil {
		return false, "", err
	}
	iv, err := semver.NewVersion(installed)
	if err != nil {
		return false, latest, nil // can't parse installed, suggest update
	}
	lv, err := semver.NewVersion(latest)
	if err != nil {
		return false, "", fmt.Errorf("parsing latest version %q: %w", latest, err)
	}
	return lv.GreaterThan(iv), latest, nil
}

// EnsureBinary downloads Otto if it's missing or below MinVersion.
func EnsureBinary() error {
	version, err := InstalledVersion()
	if err != nil {
		return err
	}
	if version == "" {
		fmt.Println("Downloading Otto agent...")
		return downloadAndInstall()
	}
	iv, err := semver.NewVersion(version)
	if err != nil {
		return downloadAndInstall()
	}
	minVer, _ := semver.NewVersion(MinVersion)
	if iv.LessThan(minVer) {
		fmt.Printf("Otto %s is below minimum required version %s, updating...\n", version, MinVersion)
		return downloadAndInstall()
	}
	return nil
}

// Update downloads and installs the latest Otto binary.
func Update() error {
	fmt.Println("Updating Otto agent...")
	return downloadAndInstall()
}

// downloadURL constructs the CDN URL for the latest Otto on this platform.
func downloadURL() string {
	goos := runtime.GOOS   // "darwin", "linux", "windows"
	arch := runtime.GOARCH // "arm64", "amd64"
	if arch == "amd64" {
		arch = "x64"
	}
	if goos == windowsGOOS {
		return fmt.Sprintf("%s/latest/%s-%s-%s.exe.zip", cdnBaseURL, binaryName, goos, arch)
	}
	return fmt.Sprintf("%s/latest/%s-%s-%s.tar.gz", cdnBaseURL, binaryName, goos, arch)
}

// downloadAndInstall fetches the Otto archive and extracts it to BinDir().
func downloadAndInstall() error {
	url := downloadURL()
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("downloading otto: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading otto: HTTP %d from %s", resp.StatusCode, url)
	}

	binDir := BinDir()
	if err := os.MkdirAll(binDir, dirPerm); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	// Save to a temp file first (needed for zip; simpler for tar.gz too)
	tmpFile, err := os.CreateTemp("", "otto-download-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("downloading otto: %w", err)
	}
	tmpFile.Close()

	if runtime.GOOS == windowsGOOS {
		if err := extractZip(tmpFile.Name(), binDir); err != nil {
			return err
		}
	} else {
		if err := extractTarGz(tmpFile.Name(), binDir); err != nil {
			return err
		}
	}

	if err := renamePlatformBinary(binDir); err != nil {
		return err
	}
	_ = os.Chmod(BinaryPath(), binPerm)

	v, _ := InstalledVersion()
	if v != "" {
		fmt.Printf("Otto %s installed\n", v)
	}
	return nil
}

// renamePlatformBinary renames the extracted `otto-<os>-<arch>` to `otto`,
// overwriting any prior version. os.Rename on POSIX replaces the target
// atomically, so an `astro agent update` reliably swaps the old binary out.
func renamePlatformBinary(binDir string) error {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("reading bin directory: %w", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), binaryName+"-") && !e.IsDir() {
			if err := os.Rename(filepath.Join(binDir, e.Name()), BinaryPath()); err != nil {
				return fmt.Errorf("installing otto binary: %w", err)
			}
			return nil
		}
	}
	return nil
}

// extractTarGz extracts a .tar.gz file to the target directory.
func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("decompressing archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}

		name := filepath.Clean(header.Name)
		if strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dst, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dirPerm); err != nil {
				return fmt.Errorf("creating directory %s: %w", name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
				return fmt.Errorf("creating parent dir for %s: %w", name, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("writing %s: %w", name, err)
			}
			if _, err := io.Copy(out, tr); err != nil { //nolint:gosec
				out.Close()
				return fmt.Errorf("writing %s: %w", name, err)
			}
			out.Close()
		}
	}
	return nil
}

// extractZip extracts a .zip file to the target directory.
func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening zip archive: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dst, name)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, dirPerm); err != nil {
				return fmt.Errorf("creating directory %s: %w", name, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
			return fmt.Errorf("creating parent dir for %s: %w", name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("writing %s: %w", name, err)
		}
		if _, err := io.Copy(out, rc); err != nil { //nolint:gosec
			out.Close()
			rc.Close()
			return fmt.Errorf("writing %s: %w", name, err)
		}
		out.Close()
		rc.Close()
	}
	return nil
}
