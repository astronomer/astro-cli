package cosmosboost

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/astronomer/astro-cli/config"
)

// Cleanup removes the Cosmos Boost artifacts under the given roots (default
// ".") and, if one is still installed, the retired standalone helper binary
// that pre-release builds of this integration downloaded.
func Cleanup(roots ...string) error {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if err := removeArtifacts(roots); err != nil {
		return err
	}
	if err := removeRetiredHelper(); err != nil {
		return err
	}
	fmt.Println("Removed the Cosmos Boost artifacts")
	return nil
}

// removeRetiredHelper deletes the standalone astro-cosmos-boost binary (and
// any partial extracts) that pre-release builds installed under the Astro
// CLI's bin directory. Nothing installs it anymore; the pre-deploy step runs
// in-process. This is migration cleanup only.
func removeRetiredHelper() error {
	binDir := filepath.Join(config.HomeConfigPath, "bin")
	name := "astro-cosmos-boost"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.Remove(filepath.Join(binDir, name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing the retired astro-cosmos-boost helper: %w", err)
	}
	if leftovers, err := filepath.Glob(filepath.Join(binDir, ".cosmosboost-*")); err == nil {
		for _, path := range leftovers {
			_ = os.Remove(path)
		}
	}
	return nil
}
