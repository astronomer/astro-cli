package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const binaryName = "astro-agent"

func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent [prompt]",
		Short: "AI-powered development agent for Airflow",
		Long:  "Start an interactive AI agent session pre-configured for Apache Airflow development.",
		RunE:  runAgent,
		// Skip the heavy astro pre-run hooks — agent doesn't need them
		Annotations: map[string]string{
			"skipPreRun": "true",
		},
		DisableFlagParsing: true, // Pass all flags through to the agent
	}
	return cmd
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Handle --help ourselves since DisableFlagParsing is set
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return cmd.Help()
		}
	}

	if len(opencodeCompressed) == 0 {
		return fmt.Errorf("agent binary not embedded — run 'make build' first")
	}

	if strings.TrimSpace(opencodeVersion) == "unsupported" {
		return fmt.Errorf("astro agent is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	binPath, err := ensureBinary()
	if err != nil {
		return fmt.Errorf("failed to set up agent: %w", err)
	}

	if _, err := ensureSkills(); err != nil {
		// Skills are non-fatal — agent still works, just without built-in skills
		fmt.Fprintf(os.Stderr, "Warning: could not extract skills: %v\n", err)
	}

	return execAgent(binPath, args)
}

// ensureBinary extracts the embedded agent tarball (binary + assets) to a cache directory.
// It only re-extracts when the checksum changes.
func ensureBinary() (string, error) {
	cacheDir, err := agentCacheDir()
	if err != nil {
		return "", err
	}

	binName := binaryName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(cacheDir, binName)
	checksumPath := filepath.Join(cacheDir, "checksum")

	// Compute checksum of the embedded tarball to detect changes
	hash := sha256.Sum256(opencodeCompressed)
	currentChecksum := hex.EncodeToString(hash[:])

	// Check if we already have the right version extracted
	if existingChecksum, err := os.ReadFile(checksumPath); err == nil {
		if strings.TrimSpace(string(existingChecksum)) == currentChecksum {
			if _, err := os.Stat(binPath); err == nil {
				return binPath, nil
			}
		}
	}

	// Extract the tarball (binary + theme files + package.json)
	fmt.Fprintf(os.Stderr, "Setting up agent...\n")

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}

	if err := extractTarGz(opencodeCompressed, cacheDir); err != nil {
		return "", fmt.Errorf("extracting agent: %w", err)
	}

	// Ensure the binary is executable
	if err := os.Chmod(binPath, 0o755); err != nil {
		return "", err
	}

	// Write checksum for future cache validation
	os.WriteFile(checksumPath, []byte(currentChecksum), 0o644) //nolint:errcheck

	return binPath, nil
}

// ensureSkills extracts the embedded skills tarball to ~/.agents/skills/.
// Pi auto-discovers skills from this directory.
func ensureSkills() (string, error) {
	if len(skillsCompressed) == 0 {
		return "", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	skillsDir := filepath.Join(home, ".agents", "skills")
	checksumPath := filepath.Join(home, ".agents", ".astro-skills-checksum")

	hash := sha256.Sum256(skillsCompressed)
	currentChecksum := hex.EncodeToString(hash[:])

	// Check if already extracted with matching checksum
	if existingChecksum, err := os.ReadFile(checksumPath); err == nil {
		if strings.TrimSpace(string(existingChecksum)) == currentChecksum {
			if _, err := os.Stat(filepath.Join(skillsDir, "index.json")); err == nil {
				return skillsDir, nil
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Updating skills...\n")

	// Remove old skills and extract fresh
	os.RemoveAll(skillsDir)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return "", err
	}

	if err := extractTarGz(skillsCompressed, skillsDir); err != nil {
		return "", fmt.Errorf("extracting skills: %w", err)
	}

	os.WriteFile(checksumPath, []byte(currentChecksum), 0o644) //nolint:errcheck

	return skillsDir, nil
}

// extractTarGz extracts a gzipped tarball into the destination directory.
func extractTarGz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Clean(header.Name)) //nolint:gosec
		// Prevent path traversal
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}

	return nil
}

// agentCacheDir returns ~/.astro/agent/ as the cache location.
func agentCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".astro", "agent"), nil
}

// BinaryPath returns the path to the extracted agent binary, if it exists.
func BinaryPath() string {
	cacheDir, err := agentCacheDir()
	if err != nil {
		return ""
	}
	binPath := filepath.Join(cacheDir, binaryName)
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}
	// Fallback to PATH
	if p, err := exec.LookPath(binaryName); err == nil {
		return p
	}
	return ""
}
