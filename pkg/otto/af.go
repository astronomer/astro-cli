package otto

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// afPinnedVersion is the astro-airflow-mcp release that Otto is developed
// against. Bump in lockstep with Otto's prompt reference.
const afPinnedVersion = "0.6.4"

// afPackageSpec is the pip-style spec passed to uvx --from.
const afPackageSpec = "astro-airflow-mcp==" + afPinnedVersion

// AfWrapperPath returns the path to the shell wrapper we install at
// `~/.astro/bin/af` (plus `.cmd` on Windows). Lives alongside the Otto binary.
func AfWrapperPath() string {
	name := "af"
	if runtime.GOOS == windowsGOOS {
		name += ".cmd"
	}
	return filepath.Join(BinDir(), name)
}

// afWrapperContent returns the wrapper script body for the current platform.
// It's a thin shim that invokes `uvx --from 'astro-airflow-mcp==X' af "$@"`.
// We write this instead of running `uv tool install` so we don't have to chase
// where uv put the binary or worry about the user's PATH including ~/.local/bin.
func afWrapperContent() string {
	if runtime.GOOS == windowsGOOS {
		return fmt.Sprintf("@echo off\r\nuvx --from %q af %%*\r\n", afPackageSpec)
	}
	return fmt.Sprintf("#!/bin/sh\nexec uvx --from '%s' af \"$@\"\n", afPackageSpec)
}

// EnsureAfWrapper writes the `af` wrapper at AfWrapperPath() if it's missing
// or out-of-date. Best-effort: returns nil on success and a diagnostic error
// otherwise — callers should log and continue rather than failing the whole
// `astro otto` launch, because Otto has a built-in fallback to the full
// `uvx --from ...` prefix when the wrapper isn't on PATH.
func EnsureAfWrapper() error {
	path := AfWrapperPath()
	desired := afWrapperContent()

	if existing, err := os.ReadFile(path); err == nil && string(existing) == desired {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(desired), binPerm); err != nil {
		return fmt.Errorf("writing af wrapper: %w", err)
	}
	return nil
}
