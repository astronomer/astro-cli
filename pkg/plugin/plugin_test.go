package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindPlugin_LongestMatch(t *testing.T) {
	// Create a temporary directory for test plugins
	tmpDir := t.TempDir()

	// Create test plugin files
	createTestPlugin(t, tmpDir, "astro-test")
	createTestPlugin(t, tmpDir, "astro-test-sub")
	createTestPlugin(t, tmpDir, "astro-test-sub-deep")

	// Add tmpDir to PATH for this test
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	tests := []struct {
		name           string
		args           []string
		expectedPlugin string
		expectedArgs   []string
		expectError    bool
	}{
		{
			name:           "longest match - three levels",
			args:           []string{"test", "sub", "deep", "arg1", "arg2"},
			expectedPlugin: "astro-test-sub-deep",
			expectedArgs:   []string{"arg1", "arg2"},
			expectError:    false,
		},
		{
			name:           "longest match - two levels",
			args:           []string{"test", "sub", "arg1"},
			expectedPlugin: "astro-test-sub",
			expectedArgs:   []string{"arg1"},
			expectError:    false,
		},
		{
			name:           "longest match - one level",
			args:           []string{"test", "arg1", "arg2"},
			expectedPlugin: "astro-test",
			expectedArgs:   []string{"arg1", "arg2"},
			expectError:    false,
		},
		{
			name:           "fallback to shorter match",
			args:           []string{"test", "sub", "notexist"},
			expectedPlugin: "astro-test-sub",
			expectedArgs:   []string{"notexist"},
			expectError:    false,
		},
		{
			name:        "no match",
			args:        []string{"notfound", "arg1"},
			expectError: true,
		},
		{
			name:        "empty args",
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginPath, pluginArgs, err := FindPlugin(tt.args)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, "", pluginPath)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, pluginPath, tt.expectedPlugin)
			assert.Equal(t, tt.expectedArgs, pluginArgs)
		})
	}
}

func TestFindPlugin_NotFound(t *testing.T) {
	args := []string{"nonexistent-plugin"}
	pluginPath, pluginArgs, err := FindPlugin(args)

	assert.ErrorIs(t, err, ErrPluginNotFound)
	assert.Equal(t, "", pluginPath)
	assert.Nil(t, pluginArgs)
}

func TestExecutePlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to different script requirements")
	}

	tmpDir := t.TempDir()

	// Create a simple test plugin that echoes its arguments
	pluginPath := filepath.Join(tmpDir, "astro-test-echo")
	pluginContent := `#!/bin/sh
echo "Plugin args: $@"
exit 0
`
	err := os.WriteFile(pluginPath, []byte(pluginContent), 0o755)
	require.NoError(t, err)

	// Test executing the plugin
	err = ExecutePlugin(pluginPath, []string{"arg1", "arg2"})
	assert.NoError(t, err)
}

func TestExecutePlugin_NonExecutable(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a non-executable file
	pluginPath := filepath.Join(tmpDir, "astro-test-nonexec")
	err := os.WriteFile(pluginPath, []byte("#!/bin/sh\necho test"), 0o644)
	require.NoError(t, err)

	// Test executing should fail
	err = ExecutePlugin(pluginPath, []string{})
	assert.ErrorIs(t, err, ErrPluginNotExecutable)
}

func TestListPlugins(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test plugins
	createTestPlugin(t, tmpDir, "astro-test1")
	createTestPlugin(t, tmpDir, "astro-test2")
	createTestPlugin(t, tmpDir, "astro-ml-train")
	// Create a non-plugin file
	nonPluginPath := filepath.Join(tmpDir, "other-binary")
	err := os.WriteFile(nonPluginPath, []byte("#!/bin/sh\necho test"), 0o755)
	require.NoError(t, err)

	// Add tmpDir to PATH for this test
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	plugins, err := ListPlugins()
	require.NoError(t, err)

	// Verify we found our test plugins (but not the non-plugin)
	pluginNames := make(map[string]bool)
	for _, p := range plugins {
		pluginNames[p.BinaryName] = true
	}

	assert.True(t, pluginNames["astro-test1"], "Should find astro-test1")
	assert.True(t, pluginNames["astro-test2"], "Should find astro-test2")
	assert.True(t, pluginNames["astro-ml-train"], "Should find astro-ml-train")
	assert.False(t, pluginNames["other-binary"], "Should not find non-plugin binary")

	// Verify plugin details
	for _, p := range plugins {
		if p.BinaryName == "astro-test1" {
			assert.Equal(t, "test1", p.Name)
			assert.Contains(t, p.Path, "astro-test1")
		}
		if p.BinaryName == "astro-ml-train" {
			assert.Equal(t, "ml-train", p.Name)
		}
	}
}

func TestListPlugins_EmptyPath(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	os.Setenv("PATH", "")

	plugins, err := ListPlugins()
	assert.NoError(t, err)
	assert.Empty(t, plugins)
}

func TestIsPluginCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid plugin", "astro-test", true},
		{"valid nested plugin", "astro-test-sub", true},
		{"not a plugin", "other-command", false},
		{"empty string", "", false},
		{"just prefix", "astro-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPluginCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommandNameFromPlugin(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"astro-ai", "ai"},
		{"astro-ml-train", "ml train"},
		{"astro-test-sub-deep", "test sub deep"},
		{"astro-", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CommandNameFromPlugin(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPluginNameFromCommand(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"ai"}, "astro-ai"},
		{[]string{"ml", "train"}, "astro-ml-train"},
		{[]string{"test", "sub", "deep"}, "astro-test-sub-deep"},
		{[]string{}, "astro-"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := PluginNameFromCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to different permission model")
	}

	tmpDir := t.TempDir()

	// Create executable file
	execPath := filepath.Join(tmpDir, "executable")
	err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755)
	require.NoError(t, err)

	// Create non-executable file
	nonExecPath := filepath.Join(tmpDir, "nonexecutable")
	err = os.WriteFile(nonExecPath, []byte("test"), 0o644)
	require.NoError(t, err)

	assert.True(t, isExecutable(execPath), "Should detect executable file")
	assert.False(t, isExecutable(nonExecPath), "Should detect non-executable file")
	assert.False(t, isExecutable("/nonexistent/path"), "Should return false for nonexistent file")
}

// Helper function to create a test plugin
func createTestPlugin(t *testing.T, dir, name string) string {
	t.Helper()

	pluginPath := filepath.Join(dir, name)
	var content string

	if runtime.GOOS == "windows" {
		content = "@echo off\necho Test plugin\n"
		pluginPath += ".bat"
	} else {
		content = "#!/bin/sh\necho \"Test plugin\"\nexit 0\n"
	}

	err := os.WriteFile(pluginPath, []byte(content), 0o755)
	require.NoError(t, err)

	return pluginPath
}

// TestFindPlugin_PathPrecedence tests that the first plugin in PATH takes precedence
func TestFindPlugin_PathPrecedence(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	// Create same plugin in both directories
	plugin1 := createTestPlugin(t, tmpDir1, "astro-precedence")
	createTestPlugin(t, tmpDir2, "astro-precedence")

	// Set PATH with tmpDir1 first
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir1+string(os.PathListSeparator)+tmpDir2)

	pluginPath, _, err := FindPlugin([]string{"precedence"})
	require.NoError(t, err)

	// Should find the one in tmpDir1 (first in PATH)
	assert.Equal(t, plugin1, pluginPath)
}

func TestFindPlugin_WithFlags(t *testing.T) {
	tmpDir := t.TempDir()
	createTestPlugin(t, tmpDir, "astro-test")

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	// Test that flags are properly passed through
	pluginPath, args, err := FindPlugin([]string{"test", "--verbose", "arg1", "--flag=value"})
	require.NoError(t, err)
	assert.Contains(t, pluginPath, "astro-test")
	assert.Equal(t, []string{"--verbose", "arg1", "--flag=value"}, args)
}

func TestListPlugins_Deduplication(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	// Create same plugin in both directories
	createTestPlugin(t, tmpDir1, "astro-duplicate")
	createTestPlugin(t, tmpDir2, "astro-duplicate")

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir1+string(os.PathListSeparator)+tmpDir2)

	plugins, err := ListPlugins()
	require.NoError(t, err)

	// Count how many times we see the duplicate plugin
	count := 0
	for _, p := range plugins {
		if p.BinaryName == "astro-duplicate" {
			count++
			// Should be from tmpDir1 (first in PATH)
			assert.Contains(t, p.Path, tmpDir1)
		}
	}

	assert.Equal(t, 1, count, "Should only list each plugin once")
}
