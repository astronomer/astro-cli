package plugin

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// PluginPrefix is the prefix for plugin binaries in PATH
	PluginPrefix = "astro-"
)

var (
	// ErrPluginNotFound is returned when no plugin is found for the given command
	ErrPluginNotFound = errors.New("plugin not found")
	// ErrPluginNotExecutable is returned when the plugin file is not executable
	ErrPluginNotExecutable = errors.New("plugin is not executable")
)

// Plugin represents a discovered plugin
type Plugin struct {
	Name       string // The plugin command name (e.g., "ai" from "astro-ai")
	BinaryName string // The full binary name (e.g., "astro-ai")
	Path       string // The full path to the plugin binary
}

// FindPlugin searches for a plugin using the longest-match algorithm.
// For a command like "astro ai foo bar", it will try to find:
// 1. astro-ai-foo-bar
// 2. astro-ai-foo
// 3. astro-ai
// Returns the plugin path and the remaining arguments to pass to the plugin.
func FindPlugin(args []string) (pluginPath string, pluginArgs []string, err error) {
	if len(args) == 0 {
		return "", nil, ErrPluginNotFound
	}

	// Try longest match first, working backwards
	for i := len(args); i > 0; i-- {
		pluginName := PluginPrefix + strings.Join(args[:i], "-")
		path, lookupErr := exec.LookPath(pluginName)
		if lookupErr == nil {
			// Found the plugin, return it with remaining args
			remainingArgs := args[i:]
			return path, remainingArgs, nil
		}
	}

	return "", nil, ErrPluginNotFound
}

// ExecutePlugin executes the plugin binary with the given arguments.
// It connects stdin, stdout, and stderr to the current process and
// preserves the plugin's exit code.
func ExecutePlugin(pluginPath string, args []string) error {
	// Verify the plugin file is executable
	if !isExecutable(pluginPath) {
		return fmt.Errorf("%w: %s", ErrPluginNotExecutable, pluginPath)
	}

	cmd := exec.Command(pluginPath, args...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	// Run and preserve exit code
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Preserve the plugin's exit code
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute plugin: %w", err)
	}

	return nil
}

// ListPlugins discovers all plugins in the system PATH.
// It searches for executables that start with the PluginPrefix.
func ListPlugins() ([]Plugin, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil, nil
	}

	paths := filepath.SplitList(pathEnv)
	pluginMap := make(map[string]Plugin) // Use map to deduplicate

	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// Skip directories we can't read
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, PluginPrefix) {
				continue
			}

			// Skip if we've already found this plugin in an earlier PATH entry
			if _, exists := pluginMap[name]; exists {
				continue
			}

			fullPath := filepath.Join(dir, name)

			// Only include executable files
			if !isExecutable(fullPath) {
				continue
			}

			// Extract command name by removing prefix
			commandName := strings.TrimPrefix(name, PluginPrefix)

			pluginMap[name] = Plugin{
				Name:       commandName,
				BinaryName: name,
				Path:       fullPath,
			}
		}
	}

	// Convert map to slice
	plugins := make([]Plugin, 0, len(pluginMap))
	for _, plugin := range pluginMap {
		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

// IsPluginCommand checks if a command name matches the plugin naming pattern
func IsPluginCommand(name string) bool {
	return strings.HasPrefix(name, PluginPrefix)
}

// isExecutable checks if a file is executable
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check if it's a regular file and has execute permissions
	mode := info.Mode()
	return mode.IsRegular() && (mode.Perm()&0o111 != 0)
}

// CommandNameFromPlugin converts a plugin binary name to a command name
// Example: "astro-ai-chat" -> "ai chat"
func CommandNameFromPlugin(binaryName string) string {
	name := strings.TrimPrefix(binaryName, PluginPrefix)
	return strings.ReplaceAll(name, "-", " ")
}

// PluginNameFromCommand converts a command to a plugin binary name
// Example: []string{"ai", "chat"} -> "astro-ai-chat"
func PluginNameFromCommand(args []string) string {
	return PluginPrefix + strings.Join(args, "-")
}
