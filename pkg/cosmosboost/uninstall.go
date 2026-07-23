package cosmosboost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// Uninstall removes the Cosmos Boost plugin and its associated files under
// the given roots (default ".").
func Uninstall(roots ...string) error {
	if len(roots) == 0 {
		roots = []string{"."}
	}

	if err := EnsureBinary(); err != nil {
		return fmt.Errorf("fetching astro-cosmos-boost: %w", err)
	}
	if err := runUninstall(roots); err != nil {
		return err
	}

	if err := os.Remove(BinaryPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", BinaryPath(), err)
	}
	if leftovers, err := filepath.Glob(filepath.Join(BinDir(), ".cosmosboost-*")); err == nil {
		for _, path := range leftovers {
			_ = os.Remove(path)
		}
	}
	fmt.Println("Uninstalled the Cosmos Boost plugin and its associated files")
	return nil
}

const usageExitCode = 2

func runUninstall(roots []string) error {
	err := execUninstall(roots)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == usageExitCode {
		if err := downloadAndInstall(); err != nil {
			return fmt.Errorf("updating astro-cosmos-boost: %w", err)
		}
		err = execUninstall(roots)
	}
	return err
}

func execUninstall(roots []string) error {
	out, err := exec.Command(BinaryPath(), append([]string{"uninstall"}, roots...)...).CombinedOutput()
	logger.Debugf("astro-cosmos-boost uninstall output:\n%s", out)
	if err != nil {
		return fmt.Errorf("running astro-cosmos-boost uninstall: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
