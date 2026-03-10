package agent

import (
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
	"syscall"

	"github.com/spf13/cobra"
)

const binaryName = "opencode"

func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent [prompt]",
		Short: "AI-powered development agent for Airflow",
		Long:  "Start an interactive AI agent session powered by opencode, pre-configured for Apache Airflow development.",
		RunE:  runAgent,
		// Skip the heavy astro pre-run hooks — agent doesn't need them
		Annotations: map[string]string{
			"skipPreRun": "true",
		},
		DisableFlagParsing: true, // Pass all flags through to opencode
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
		return fmt.Errorf("opencode binary not embedded — run 'make embed-opencode' first")
	}

	binPath, err := ensureBinary()
	if err != nil {
		return fmt.Errorf("failed to extract opencode binary: %w", err)
	}

	// Use syscall.Exec to replace this process with opencode.
	// This gives opencode full control of the terminal (TUI, signals, etc).
	argv := append([]string{binaryName}, args...)
	env := os.Environ()

	return syscall.Exec(binPath, argv, env)
}

// ensureBinary extracts the embedded opencode binary to a cache directory.
// It only re-extracts when the version or checksum changes.
func ensureBinary() (string, error) {
	cacheDir, err := agentCacheDir()
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(cacheDir, binaryName)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	checksumPath := filepath.Join(cacheDir, "checksum")

	// Compute checksum of the embedded binary to detect changes
	hash := sha256.Sum256(opencodeCompressed)
	currentChecksum := hex.EncodeToString(hash[:])

	// Check if we already have the right version extracted
	if existingChecksum, err := os.ReadFile(checksumPath); err == nil {
		if strings.TrimSpace(string(existingChecksum)) == currentChecksum {
			// Verify the binary still exists
			if _, err := os.Stat(binPath); err == nil {
				return binPath, nil
			}
		}
	}

	// Extract
	fmt.Fprintf(os.Stderr, "Extracting opencode %s...\n", strings.TrimSpace(opencodeVersion))

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}

	r, err := gzip.NewReader(bytes.NewReader(opencodeCompressed))
	if err != nil {
		return "", fmt.Errorf("decompressing opencode: %w", err)
	}
	defer r.Close()

	// Write to a temp file first, then rename for atomicity
	tmpFile, err := os.CreateTemp(cacheDir, "opencode-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	// Write checksum for future cache validation
	os.WriteFile(checksumPath, []byte(currentChecksum), 0o644) //nolint:errcheck

	return binPath, nil
}

// agentCacheDir returns ~/.astro/agent/ as the cache location.
func agentCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".astro", "agent"), nil
}

// BinaryPath returns the path to the extracted opencode binary, if it exists.
// Useful for other commands that may want to check if the agent is available.
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
